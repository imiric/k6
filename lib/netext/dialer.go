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
	"sync/atomic"
	"time"

	"github.com/loadimpact/k6/lib"
	"github.com/loadimpact/k6/lib/metrics"
	"github.com/loadimpact/k6/stats"

	"github.com/domainr/dnsr"
)

// Dialer wraps net.Dialer and provides k6 specific functionality -
// tracing, blacklists and DNS cache and aliases.
type Dialer struct {
	net.Dialer

	Resolver  *dnsr.Resolver
	Metacache map[string]bool // IPv4 last seen
	Blacklist []*lib.IPNet
	Hosts     map[string]net.IP

	BytesRead    int64
	BytesWritten int64
}

// NewDialer constructs a new Dialer and initializes its cache.
func NewDialer(dialer net.Dialer) *Dialer {
	return &Dialer{
		Dialer:    dialer,
		Resolver:  dnsr.NewExpiring(0),
		Metacache: make(map[string]bool),
	}
}

// BlackListedIPError is an error that is returned when a given IP is blacklisted
type BlackListedIPError struct {
	ip  net.IP
	net *lib.IPNet
}

func (b BlackListedIPError) Error() string {
	return fmt.Sprintf("IP (%s) is in a blacklisted range (%s)", b.ip, b.net)
}

// DialContext wraps the net.Dialer.DialContext and handles the k6 specifics
func (d *Dialer) DialContext(ctx context.Context, proto, addr string) (net.Conn, error) {
	delimiter := strings.LastIndex(addr, ":")
	host := addr[:delimiter]

	// lookup for domain defined in Hosts option before trying to resolve DNS.
	ip, ok := d.Hosts[host]
	if !ok {
		var err error
		ip, err = d.resolve(host)
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

// resolve converts a host string into an IP address.
// Host string may be an IP address string or a domain name.
func (d *Dialer) resolve(host string) (net.IP, error) {
	ip := net.ParseIP(host)
	if ip == nil {
		return d.lookup(host)
	} else {
		return ip, nil
	}
}

// lookup attempts name resolution in the most efficient order.
// Prefers IPv4 if last resolution produced it.
// Otherwise prefers IPv6.
func (d *Dialer) lookup(host string) (net.IP, error) {
	if d.Metacache[host] {
		return d.lookup4(host)
	} else {
		return d.lookup6(host)
	}
}

// lookup6 attempts to resolve to IPv6 then IPv4.
// Used on first resolution, if last resolution failed,
// or if last resolution produced IPv6.
func (d *Dialer) lookup6(host string) (net.IP, error) {
	ips, err := d.Resolver.ResolveErr(host, "AAAA")
	if err != nil {
		return net.IPv6zero, err
	}
	if len(ips) > 0 {
		ip := net.ParseIP(ips[0].Value)
		if ip != nil {
			return ip, nil
		}
	}
	ips, err = d.Resolver.ResolveErr(host, "A")
	if err != nil {
		return net.IPv4zero, err
	}
	if len(ips) > 0 {
		ip := net.ParseIP(ips[0].Value)
		if ip != nil {
			d.Metacache[host] = true
			return ip, nil
		}
	}
	return net.IPv4zero, errors.New("unable to resolve host address `" + host + "`")
}

// lookup4 attempts to resolve to IPv4 then IPv6.
// Used if last resolution produced IPv4.
// Prevents hitting network looking for IPv6 for names with only IPv4.
func (d *Dialer) lookup4(host string) (net.IP, error) {
	ips, err := d.Resolver.ResolveErr(host, "A")
	if err != nil {
		return net.IPv4zero, err
	}
	if len(ips) > 0 {
		ip := net.ParseIP(ips[0].Value)
		if ip != nil {
			return ip, nil
		}
	}
	d.Metacache[host] = false
	ips, err = d.Resolver.ResolveErr(host, "AAAA")
	if err != nil {
		return net.IPv6zero, err
	}
	if len(ips) > 0 {
		ip := net.ParseIP(ips[0].Value)
		if ip != nil {
			return ip, nil
		}
	}
	return net.IPv6zero, errors.New("unable to resolve host address `" + host + "`")
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
