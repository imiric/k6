/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2020 Load Impact
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
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/semihalev/sdns/authcache"
	"github.com/semihalev/sdns/cache"
	"github.com/semihalev/sdns/config"
	cachem "github.com/semihalev/sdns/middleware/cache"
	sdns "github.com/semihalev/sdns/middleware/resolver"

	netstd "github.com/loadimpact/k6/lib/netext/net"
)

var (
	ip4, ip6    bool
	nsm         = &sync.RWMutex{}
	nameservers []string
)

// BaseResolver is the low level DNS resolution interface.
type BaseResolver interface {
	Resolve(context.Context, *dns.Msg) (*dns.Msg, error)
}

// TODO: This is only needed for testing. Move to test package?
func SetNS(nss []string) {
	nsm.Lock()
	defer nsm.Unlock()
	nameservers = nss
}

// NewResolver returns a new DNS resolver with a preconfigured TTL-based cache
// using the given base resolver. If nil, the sdns resolver will be used.
func NewResolver(base BaseResolver) *Resolver {
	nsm.RLock()
	defer nsm.RUnlock()
	cfg := new(config.Config)
	cfg.RootServers = nameservers
	// TODO: Make this configurable?
	cfg.Maxdepth = 30
	cfg.Expire = 600
	cfg.CacheSize = 1024
	cfg.Timeout.Duration = 2 * time.Second

	if base == nil {
		base = newSdnsResolver(cfg)
	}

	return &Resolver{
		BaseResolver: base,
		cache:        cachem.New(cfg),
		ip4:          make(map[string]bool),
		cname:        make(map[string]canonicalName),
	}
}

type Resolver struct {
	BaseResolver
	ctx         context.Context
	authservers *authcache.AuthServers
	cache       *cachem.Cache
	ip4         map[string]bool // IPv4 last seen
	cname       map[string]canonicalName
}

// canonicalName is an expiring CNAME value.
type canonicalName struct {
	Name   string
	TTL    time.Duration
	Expiry time.Time
}

type sdnsResolver struct {
	*sdns.Resolver
	authservers *authcache.AuthServers
}

func newSdnsResolver(cfg *config.Config) BaseResolver {
	nsm.RLock()
	defer nsm.RUnlock()
	authservers := &authcache.AuthServers{}
	authservers.Zone = "." // should this be dynamic?
	for _, ns := range nameservers {
		host, _, _ := net.SplitHostPort(ns)
		if ip := net.ParseIP(host); ip != nil {
			authservers.List = append(authservers.List, authcache.NewAuthServer(ns, authcache.IPv4))
		}
	}

	return &sdnsResolver{
		Resolver:    sdns.NewResolver(cfg),
		authservers: authservers,
	}
}

func (r *sdnsResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	return r.Resolver.Resolve(ctx, req, r.authservers, false, 30, 0, false, nil)
}

// Resolve maps a host string to an IP address.
// Host string may be an IP address string or a domain name.
// Follows CNAME chain up to depth steps.
func (r *Resolver) Resolve(ctx context.Context, host string, depth uint8) (net.IP, error) {
	r.ctx = ctx
	ip := net.ParseIP(host)
	if ip != nil {
		return ip, nil
	}
	if ip = r.resolveLocal(host); ip != nil {
		return ip, nil
	}
	host, err := r.canonicalName(host, depth)
	if err != nil {
		return nil, err
	}
	observed := make(map[string]struct{})
	return r.resolveName(host, host, depth, observed)
}

// lookup performs a single lookup in the most efficient order.
// Prefers IPv4 if last resolution produced it.
// Otherwise prefers IPv6.
// Package config constrains to only IP versions available on the system.
func (r *Resolver) lookup(host string) (net.IP, dns.RR, error) {
	if ip6 && ip4 {
		// Both versions available
		if r.ip4[host] {
			return r.lookup46(host)
		}
		return r.lookup64(host)
	}
	if ip6 {
		// Only v6 available
		ip, cname, err := r.lookup6(host)
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
		ip, cname, err := r.lookup4(host)
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
func (r *Resolver) lookup64(host string) (net.IP, dns.RR, error) {
	ip, cname, err := r.lookup6(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		return ip, nil, nil
	}
	if cname != nil {
		return nil, cname, nil
	}
	ip, cname, err = r.lookup4(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		r.ip4[host] = true
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
func (r *Resolver) lookup46(host string) (net.IP, dns.RR, error) {
	ip, cname, err := r.lookup4(host)
	if err != nil {
		return nil, nil, err
	}
	if ip != nil {
		return ip, nil, nil
	}
	if cname != nil {
		return nil, cname, nil
	}
	r.ip4[host] = false
	ip, cname, err = r.lookup6(host)
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
func (r *Resolver) lookup6(host string) (net.IP, dns.RR, error) {
	req := makeReq(host, dns.TypeA)
	key := cache.Hash(req.Question[0])
	resp, _, err := r.cache.GetP(key, req)
	if resp == nil || err != nil {
		resp, err = r.BaseResolver.Resolve(r.ctx, req)
		if resp != nil {
			r.cache.Set(key, resp)
		}
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
func (r *Resolver) lookup4(host string) (net.IP, dns.RR, error) {
	req := makeReq(host, dns.TypeA)
	key := cache.Hash(req.Question[0])
	resp, _, err := r.cache.GetP(key, req)
	if resp == nil || err != nil {
		resp, err = r.BaseResolver.Resolve(r.ctx, req)
		if resp != nil {
			r.cache.Set(key, resp)
		}
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

// resolveName maps a domain name to an IP address.
// Follows CNAME chain up to depth steps.
// Fails on CNAME chain cycle.
func (r *Resolver) resolveName(
	requested string,
	name string,
	depth uint8,
	observed map[string]struct{},
) (net.IP, error) {
	ip, cname, err := r.lookup(name)
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
	r.cname[name] = canonicalName{
		Name:   cn.Target,
		TTL:    ttl,
		Expiry: expiry,
	}
	return r.resolveName(requested, cn.Target, depth-1, observed)
}

// canonicalName reports the best current knowledge about a canonical name.
// Follows CNAME chain up to depth steps.
// Purges expired CNAME entries.
// Fails on a cycle.
func (r *Resolver) canonicalName(name string, depth uint8) (string, error) {
	cname := normalName(name)
	observed := make(map[string]struct{})
	observed[cname] = struct{}{}
	now := time.Now()
	for entry, ok := r.cname[cname]; ok; entry, ok = r.cname[cname] {
		if now.After(entry.Expiry) {
			// Expired entry
			delete(r.cname, cname)
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

// resolveLocal attempts a local DNS resolution from /etc/hosts, but only if
// "files" is configured for it in /etc/nsswitch.conf . It returns a random
// IP if more than one is defined.
func (r *Resolver) resolveLocal(host string) net.IP {
	nss := netstd.SystemConf().NSS
	if nss.Err != nil {
		return nil
	}

	var readFiles bool
	if hosts, ok := nss.Sources["hosts"]; !ok {
		return nil
	} else {
		// Is this check sufficient? Do we need to read other entries, handle
		// failover, etc.?
		for _, h := range hosts {
			if h.Source == "files" {
				readFiles = true
				break
			}
		}
	}

	if readFiles {
		ips := netstd.LookupStaticHost(host)
		if len(ips) > 0 {
			// TODO: Something more sophisticated than random?
			ip := ips[rand.Intn(len(ips))]
			return net.ParseIP(ip)
		}
	}

	return nil
}

// calculateExpiry calculates the expiry time of an RR.
// Copied from github.com/domainr/dnsr/rr.go
func calculateExpiry(drr dns.RR) (time.Duration, time.Time) {
	ttl := time.Second * time.Duration(drr.Header().Ttl)
	expiry := time.Now().Add(ttl)
	return ttl, expiry
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

// extractIP4 returns an IPv4 address extracted from rr or nil if record is not type A.
func extractIP4(rr dns.RR) net.IP {
	if r, ok := rr.(*dns.A); ok {
		return r.A
	}
	return nil
}

// extractIP6 returns an IPv6 address extracted from rr or nil if record is not type AAAA.
func extractIP6(rr dns.RR) net.IP {
	if r, ok := rr.(*dns.AAAA); ok {
		return r.AAAA
	}
	return nil
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

// normalName normalizes a domain name.
func normalName(name string) string {
	return dns.Fqdn(strings.ToLower(name))
}

func makeReq(hostname string, dnsType uint16) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(hostname, dnsType)
	req.RecursionDesired = true
	return req
}

func init() {
	rand.Seed(time.Now().UnixNano())
	detectInterfaces()
	// TODO: Log any file parsing errors.
	nameservers = netstd.SystemConf().Resolv.Servers
}
