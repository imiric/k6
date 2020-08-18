package netext

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/miekg/dns"
	"github.com/semihalev/sdns/cache"
	"github.com/semihalev/sdns/config"
	cachem "github.com/semihalev/sdns/middleware/cache"
	"github.com/stretchr/testify/require"
)

// mockResolver implements both netext.Resolver and netext.baseResolver
type mockResolver struct {
	hosts map[string]net.IP
	cache *cache.Cache
}

func (r *mockResolver) Resolve(_ context.Context, host string, _ uint8) (resp *dns.Msg, err error) {
	req := makeReq(host, dns.TypeA)
	key := cache.Hash(req.Question[0])
	if val, ok := r.cache.Get(key); !ok {
		resp, err = r.resolve(nil, req)
		if resp != nil {
			r.cache.Add(key, resp)
		}
		if err != nil {
			return nil, err
		}
	} else {
		resp = val.(*dns.Msg)
	}
	return
}

func (r *mockResolver) resolve(_ context.Context, req *dns.Msg) (resp *dns.Msg, err error) {
	resp = new(dns.Msg)
	resp.SetReply(req)
	host := req.Question[0].Name
	if ip, ok := r.hosts[host]; ok {
		var rtype string
		if ip.To4() == nil {
			rtype = "AAAA"
		} else {
			rtype = "A"
		}
		rs := fmt.Sprintf("%s 5 IN %s %s", host, rtype, ip)
		rr, err := dns.NewRR(rs)
		if err != nil {
			return nil, err
		}
		resp.Answer = append(resp.Answer, rr)
	}
	return resp, nil
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		cache: cache.New(1024),
		hosts: map[string]net.IP{
			"host4.test.": net.ParseIP("127.0.0.1"),
			"host6.test.": net.ParseIP("::1"),
		},
	}
}

func newTestResolver() *resolver {
	cfg := new(config.Config)
	cfg.Expire = 600
	cfg.CacheSize = 1024

	return &resolver{
		baseResolver: newMockResolver(),
		cache:        cachem.New(cfg),
		ip4:          make(map[string]bool),
		cname:        make(map[string]canonicalName),
	}
}

func TestLookup(t *testing.T) {
	t.Run("never resolved", func(t *testing.T) {
		r := newTestResolver()
		require.False(t, r.ip4["example.com."])
	})

	t.Run("resolution failure", func(t *testing.T) {
		r := newTestResolver()
		_, _, err := r.lookup("example.badtld.")
		require.Error(t, err)
		require.False(t, r.ip4["example.badtld."])
	})

	t.Run("find ipv6", func(t *testing.T) {
		r := newTestResolver()
		ip, _, err := r.lookup("host6.test.")
		require.NoError(t, err)
		require.True(t, ip.To4() == nil)
		require.False(t, r.ip4["host6.test."])
	})

	t.Run("find ipv4", func(t *testing.T) {
		r := newTestResolver()
		ip, _, err := r.lookup("host4.test.")
		require.NoError(t, err)
		require.True(t, ip.To4() != nil)
		require.True(t, r.ip4["host4.test."])
	})
}

// TODO: Implement mock for this?
// func TestResolution(t *testing.T) {
// 	t.Run("follow CNAMEs", func(t *testing.T) {
// 		ip6 = true
// 		ip4 = true
// 		r := newTestResolver()
// 		ip, err := r.Resolve(r.ctx, "http2.akamai.com", 3)
// 		require.NoError(t, err)
// 		require.True(t, ip.To4() != nil)
// 		cname := r.cname["http2.akamai.com."]
// 		require.NotEqual(t, cname, nil)
// 		require.Equal(t, "http2.edgekey.net.", cname.Name)
// 	})
// }

func TestMain(m *testing.M) {
	exitCode := m.Run()
	os.Exit(exitCode)
	// reset network interfaces detection
	ip6 = true
	ip4 = true
}
