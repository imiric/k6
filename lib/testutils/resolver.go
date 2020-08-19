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

package testutils

import (
	"context"
	"fmt"
	"sync"

	"github.com/miekg/dns"
)

// MockResolver implements netext.BaseResolver, and allows changing the defined
// hosts at runtime.
type MockResolver struct {
	m *sync.Mutex
	// Mapping of FQDNs including ending period to partial DNS resource records.
	// E.g. "example.com.": "5 IN A 127.0.0.1"
	hosts map[string]string
}

func NewMockResolver(hosts map[string]string) *MockResolver {
	if hosts == nil {
		hosts = make(map[string]string)
	}
	return &MockResolver{&sync.Mutex{}, hosts}
}

func (r *MockResolver) Resolve(_ context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp = new(dns.Msg)
	resp.SetReply(req)
	host := req.Question[0].Name
	if rrs, ok := r.hosts[host]; ok {
		rr, err := dns.NewRR(fmt.Sprintf("%s %s", host, rrs))
		if err != nil {
			return nil, err
		}
		resp.Answer = append(resp.Answer, rr)
	}
	return resp, nil
}

func (r *MockResolver) SetRR(host, rr string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.hosts[host] = rr
}
