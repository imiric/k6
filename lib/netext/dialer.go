/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2016 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package netext

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loadimpact/k6/lib"
	"github.com/loadimpact/k6/lib/metrics"
	"github.com/loadimpact/k6/stats"

	"github.com/miekg/dns"
	"github.com/semihalev/sdns/authcache"
	"github.com/semihalev/sdns/cache"
	sdnsc "github.com/semihalev/sdns/config"
	cachem "github.com/semihalev/sdns/middleware/cache"
	sdns "github.com/semihalev/sdns/middleware/resolver"
)

var ip4, ip6 bool

var (
	authcacheServers *authcache.AuthServers
	// TODO: Read this from /etc/resolv.conf
	nameservers = []string{
		"1.1.1.1:53",
		"8.8.8.8:53",
	}
	once sync.Once
)

// Dialer wraps net.Dialer and provides k6 specific functionality -
// tracing, blacklists and DNS cache and aliases.
type Dialer struct {
	net.Dialer

	Resolver *sdns.Resolver
	// Unexport this
	DNSCache  *cachem.Cache
	ctx       context.Context
	IP4       map[string]bool // IPv4 last seen
	CNAME     map[string]CanonicalName
	Blacklist []*lib.IPNet
	Hosts     map[string]net.IP

	BytesRead    int64
	BytesWritten int64
}

// CanonicalName is an expiring CNAME value.
type CanonicalName struct {
	Name   string
	TTL    time.Duration
	Expiry time.Time
}

// NewDialer constructs a new Dialer and initializes its cache.
func NewDialer(dialer net.Dialer) *Dialer {
	return &Dialer{
		Dialer:   dialer,
		Resolver: NewResolver(),
		DNSCache: cachem.New(ResolverConfig()),
		IP4:      make(map[string]bool),
		CNAME:    make(map[string]CanonicalName),
	}
}

func NewResolver() *sdns.Resolver {
	return sdns.NewResolver(ResolverConfig())
}

// BlackListedIPError is an error that is returned when a given IP is blacklisted
type BlackListedIPError struct {
	ip  net.IP
	net *lib.IPNet
}

func (b BlackListedIPError) Error() string {
	return fmt.Sprintf("IP (%s) is in a blacklisted range (%s)", b.ip, b.net)
}

func authServers() *authcache.AuthServers {
	once.Do(func() {
		servers := &authcache.AuthServers{}
		servers.Zone = "."
		for _, ns := range nameservers {
			host, _, _ := net.SplitHostPort(ns)
			if ip := net.ParseIP(host); ip != nil {
				servers.List = append(servers.List, authcache.NewAuthServer(ns, authcache.IPv4))
			}
		}
		authcacheServers = servers
	})
	return authcacheServers
}

func ResolverConfig() *sdnsc.Config {
	cfg := new(sdnsc.Config)
	cfg.RootServers = nameservers
	cfg.Maxdepth = 30
	cfg.Expire = 600
	cfg.CacheSize = 1024
	cfg.Timeout.Duration = 2 * time.Second
	return cfg
}

func makeDNSReq(hostname string, dnsType uint16) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(hostname, dnsType)
	req.RecursionDesired = true
	return req
}

// calculateExpiry calculates the expiry time of an RR.
// Copied from github.com/domainr/dnsr/rr.go
func calculateExpiry(drr dns.RR) (time.Duration, time.Time) {
	ttl := time.Second * time.Duration(drr.Header().Ttl)
	expiry := time.Now().Add(ttl)
	return ttl, expiry
}

// detectInterfaces detects available IP network versions.
func detectInterfaces() {
	ip6 = false
	ip4 = false
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		panic(err)
	}
	for _, abstract := range addresses {
		switch concrete := abstract.(type) {
		case *net.IPNet:
			detectInterface(concrete.IP)
		case *net.IPAddr:
			detectInterface(concrete.IP)
		}
	}
}

// detectInterface detects an available IP network interface.
// Records the IP version available in package config.
func detectInterface(address net.IP) {
	if !address.IsUnspecified() && !address.IsLoopback() {
		if address.To4() == nil {
			ip6 = true
		}
		ip4 = true
	}
}

// DialContext wraps the net.Dialer.DialContext and handles the k6 specifics
func (d *Dialer) DialContext(ctx context.Context, proto, addr string) (net.Conn, error) {
	d.ctx = ctx
	delimiter := strings.LastIndex(addr, ":")
	host := addr[:delimiter]

	// lookup for domain defined in Hosts option before trying to resolve DNS.
	ip, ok := d.Hosts[host]
	if !ok {
		var err error
		ip, err = d.resolve(host, 10)
		if err != nil {
			return nil, err
		}
	}

	for _, ipnet := range d.Blacklist {
		if (*net.IPNet)(ipnet).Contains(ip) {
			return nil, BlackListedIPError{ip: ip, net: ipnet}
		}
	}
	ipStr := ip.String()
	if strings.ContainsRune(ipStr, ':') {
		ipStr = "[" + ipStr + "]"
	}
	conn, err := d.Dialer.DialContext(ctx, proto, ipStr+":"+addr[delimiter+1:])
	if err != nil {
		return nil, err
	}
	conn = &Conn{conn, &d.BytesRead, &d.BytesWritten}
	return conn, err
}

// resolve maps a host string to an IP address.
// Host string may be an IP address string or a domain name.
// Follows CNAME chain up to depth steps.
func (d *Dialer) resolve(host string, depth uint8) (net.IP, error) {
	ip := net.ParseIP(host)
	if ip != nil {
		return ip, nil
	}
	host, err := d.canonicalName(host, depth)
	if err != nil {
		return nil, err
	}
	observed := make(map[string]struct{})
	return d.resolveName(host, host, depth, observed)
}

// resolveName maps a domain name to an IP address.
// Follows CNAME chain up to depth steps.
// Fails on CNAME chain cycle.
func (d *Dialer) resolveName(
	requested string,
	name string,
	depth uint8,
	observed map[string]struct{},
) (net.IP, error) {
	ip, cname, err := d.lookup(name)
	if err != nil {
		// Lookup error
		return nil, err
	}
	if ip != nil {
		// Found IP address
		return ip, nil
	}
	if cname == nil {
		// Found nothing
		return nil, errors.New("unable to resolve host address `" + requested + "`")
	}
	if depth == 0 {
		// Long CNAME chain
		return nil, errors.New("CNAME chain too long for `" + requested + "`")
	}
	var (
		cn *dns.CNAME
		ok bool
	)
	if cn, ok = cname.(*dns.CNAME); !ok {
		return nil, fmt.Errorf("expected *dns.CNAME, received: %T", cname)
	}

	if _, ok := observed[cn.Target]; ok {
		// CNAME chain cycle
		return nil, errors.New("cycle in CNAME chain for `" + requested + "`")
	}
	// Found CNAME
	observed[cn.Target] = struct{}{}
	ttl, expiry := calculateExpiry(cname)
	d.CNAME[name] = CanonicalName{
		Name:   cn.Target,
		TTL:    ttl,
		Expiry: expiry,
	}
	return d.resolveName(requested, cn.Target, depth-1, observed)
}

// canonicalName reports the best current knowledge about a canonical name.
// Follows CNAME chain up to depth steps.
// Purges expired CNAME entries.
// Fails on a cycle.
func (d *Dialer) canonicalName(name string, depth uint8) (string, error) {
	cname := normalName(name)
	observed := make(map[string]struct{})
	observed[cname] = struct{}{}
	now := time.Now()
	for entry, ok := d.CNAME[cname]; ok; entry, ok = d.CNAME[cname] {
		if now.After(entry.Expiry) {
			// Expired entry
			delete(d.CNAME, cname)
			return cname, nil
		}
		if depth == 0 {
			// Long chain
			return "", errors.New("CNAME chain too long for `" + name + "`")
		}
		cname = entry.Name
		if _, ok := observed[cname]; ok {
			// Cycle
			return "", errors.New("cycle in CNAME chain for `" + name + "`")
		}
		observed[cname] = struct{}{}
		depth--
	}
	return cname, nil
}

// lookup performs a single lookup in the most efficient order.
// Prefers IPv4 if last resolution produced it.
// Otherwise prefers IPv6.
// Package config constrains to only IP versions available on the system.
func (d *Dialer) lookup(host string) (net.IP, dns.RR, error) {
	if ip6 && ip4 {
		// Both versions available
		if d.IP4[host] {
			return d.lookup46(host)
		}
		return d.lookup64(host)
	}
	if ip6 {
		// Only v6 available
		ip, cname, err := d.lookup6(host)
		if err != nil {
			return nil, nil, err
		}
		if ip != nil {
			return ip, nil, nil
		}
		if cname != nil {
			return nil, cname, nil
		}
		return nil, nil, errors.New("unable to resolve host address `" + host + "`")
	}
	if ip4 {
		// Only v4 available
		ip, cname, err := d.lookup4(host)
		if err != nil {
			return nil, nil, err
		}
		if ip != nil {
			return ip, nil, nil
		}
		if cname != nil {
			return nil, cname, nil
		}
		return nil, nil, errors.New("unable to resolve host address `" + host + "`")
	}
	// Neither version available
	return nil, nil, errors.New("network interface unavailable")
}

// lookup64 performs a single lookup preferring IPv6.
// Used on first resolution, if last resolution failed,
// or if last resolution produced IPv6.
func (d *Dialer) lookup64(host string) (net.IP, dns.RR, error) {
	ip, cname, err := d.lookup6(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		return ip, nil, nil
	}
	if cname != nil {
		return nil, cname, nil
	}
	ip, cname, err = d.lookup4(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		d.IP4[host] = true
		return ip, nil, nil
	}
	if cname != nil {
		return nil, cname, nil
	}
	return nil, nil, errors.New("unable to resolve host address `" + host + "`")
}

// lookup46 performs a single lookup preferring IPv4.
// Used if last resolution produced IPv4.
// Prevents hitting network looking for IPv6 for names with only IPv4.
func (d *Dialer) lookup46(host string) (net.IP, dns.RR, error) {
	ip, cname, err := d.lookup4(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		return ip, nil, nil
	}
	if cname != nil {
		return nil, cname, nil
	}
	d.IP4[host] = false
	ip, cname, err = d.lookup6(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		return ip, nil, nil
	}
	if cname != nil {
		return nil, cname, nil
	}
	return nil, nil, errors.New("unable to resolve host address `" + host + "`")
}

// lookup6 performs a single lookup for IPv6.
func (d *Dialer) lookup6(host string) (net.IP, dns.RR, error) {
	req := makeDNSReq(host, dns.TypeA)
	key := cache.Hash(req.Question[0])
	resp, _, err := d.DNSCache.GetP(key, req)
	if resp == nil || err != nil {
		resp, err = d.Resolver.Resolve(d.ctx, req, authServers(), false, 30, 0, false, nil)
		d.DNSCache.Set(key, resp)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(resp.Answer) > 0 {
		ip, cname := findIP6(resp.Answer)
		if ip != nil {
			return ip, nil, nil
		}
		if cname != nil {
			return nil, cname, nil
		}
	}
	return nil, nil, nil
}

// lookup4 performs a single lookup for IPv4.
func (d *Dialer) lookup4(host string) (net.IP, dns.RR, error) {
	req := makeDNSReq(host, dns.TypeA)
	key := cache.Hash(req.Question[0])
	resp, _, err := d.DNSCache.GetP(key, req)
	if resp == nil || err != nil {
		resp, err = d.Resolver.Resolve(d.ctx, req, authServers(), false, 30, 0, false, nil)
		d.DNSCache.Set(key, resp)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(resp.Answer) > 0 {
		ip, cname := findIP4(resp.Answer)
		if ip != nil {
			return ip, nil, nil
		}
		if cname != nil {
			return nil, cname, nil
		}
	}
	return nil, nil, nil
}

// findIP6 returns the first IPv6 address found in rrs.
// Alternately returns a CNAME record if found.
func findIP6(rrs []dns.RR) (net.IP, dns.RR) {
	var cname dns.RR = nil
	for _, rr := range rrs {
		if ip := extractIP6(rr); ip != nil {
			return ip, nil
		}
		if rr.Header().Rrtype == dns.TypeCNAME {
			cname = rr
		}
	}
	return nil, cname
}

// findIP4 returns the first IPv4 address found in rrs.
// Alternately returns a CNAME record if found.
func findIP4(rrs []dns.RR) (net.IP, dns.RR) {
	var cname dns.RR = nil
	for _, rr := range rrs {
		if ip := extractIP4(rr); ip != nil {
			return ip, nil
		}
		if rr.Header().Rrtype == dns.TypeCNAME {
			cname = rr
		}
	}
	return nil, cname
}

// extractIP6 returns an IPv6 address extracted from rr or nil if record is not type AAAA.
func extractIP6(rr dns.RR) net.IP {
	if r, ok := rr.(*dns.AAAA); ok {
		return r.AAAA
	}
	return nil
}

// extractIP4 returns an IPv4 address extracted from rr or nil if record is not type A.
func extractIP4(rr dns.RR) net.IP {
	if r, ok := rr.(*dns.A); ok {
		return r.A
	}
	return nil
}

// normalName normalizes a domain name.
func normalName(name string) string {
	return dns.Fqdn(strings.ToLower(name))
}

// GetTrail creates a new NetTrail instance with the Dialer
// sent and received data metrics and the supplied times and tags.
// TODO: Refactor this according to
// https://github.com/loadimpact/k6/pull/1203#discussion_r337938370
func (d *Dialer) GetTrail(
	startTime, endTime time.Time, fullIteration bool, emitIterations bool, tags *stats.SampleTags,
) *NetTrail {
	bytesWritten := atomic.SwapInt64(&d.BytesWritten, 0)
	bytesRead := atomic.SwapInt64(&d.BytesRead, 0)
	samples := []stats.Sample{
		{
			Time:   endTime,
			Metric: metrics.DataSent,
			Value:  float64(bytesWritten),
			Tags:   tags,
		},
		{
			Time:   endTime,
			Metric: metrics.DataReceived,
			Value:  float64(bytesRead),
			Tags:   tags,
		},
	}
	if fullIteration {
		samples = append(samples, stats.Sample{
			Time:   endTime,
			Metric: metrics.IterationDuration,
			Value:  stats.D(endTime.Sub(startTime)),
			Tags:   tags,
		})
		if emitIterations {
			samples = append(samples, stats.Sample{
				Time:   endTime,
				Metric: metrics.Iterations,
				Value:  1,
				Tags:   tags,
			})
		}
	}

	return &NetTrail{
		BytesRead:     bytesRead,
		BytesWritten:  bytesWritten,
		FullIteration: fullIteration,
		StartTime:     startTime,
		EndTime:       endTime,
		Tags:          tags,
		Samples:       samples,
	}
}

// NetTrail contains information about the exchanged data size and length of a
// series of connections from a particular netext.Dialer
type NetTrail struct {
	BytesRead     int64
	BytesWritten  int64
	FullIteration bool
	StartTime     time.Time
	EndTime       time.Time
	Tags          *stats.SampleTags
	Samples       []stats.Sample
}

// Ensure that interfaces are implemented correctly
var _ stats.ConnectedSampleContainer = &NetTrail{}

// GetSamples implements the stats.SampleContainer interface.
func (ntr *NetTrail) GetSamples() []stats.Sample {
	return ntr.Samples
}

// GetTags implements the stats.ConnectedSampleContainer interface.
func (ntr *NetTrail) GetTags() *stats.SampleTags {
	return ntr.Tags
}

// GetTime implements the stats.ConnectedSampleContainer interface.
func (ntr *NetTrail) GetTime() time.Time {
	return ntr.EndTime
}

// Conn wraps net.Conn and keeps track of sent and received data size
type Conn struct {
	net.Conn

	BytesRead, BytesWritten *int64
}

func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		atomic.AddInt64(c.BytesRead, int64(n))
	}
	return n, err
}

func (c *Conn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		atomic.AddInt64(c.BytesWritten, int64(n))
	}
	return n, err
}

// init detects available IP network versions.
func init() {
	detectInterfaces()
}
