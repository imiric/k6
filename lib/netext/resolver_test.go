package netext

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/loadimpact/k6/lib/testutils"
)

func newTestResolver() *Resolver {
	hosts := map[string]string{
		"host4.test.": "5 IN A 127.0.0.1",
		"host6.test.": "5 IN AAAA ::1",
	}
	baseResolver := testutils.NewMockResolver(hosts)
	return NewResolver(baseResolver)
}

func TestLookup(t *testing.T) {
	t.Run("never resolved", func(t *testing.T) {
		r := newTestResolver()
		assert.False(t, r.ip4["example.com."])
	})

	t.Run("resolution failure", func(t *testing.T) {
		r := newTestResolver()
		_, _, err := r.lookup("example.badtld.")
		require.Error(t, err)
		assert.False(t, r.ip4["example.badtld."])
	})

	t.Run("find ipv6", func(t *testing.T) {
		r := newTestResolver()
		ip, _, err := r.lookup("host6.test.")
		require.NoError(t, err)
		assert.True(t, ip.To4() == nil)
		assert.False(t, r.ip4["host6.test."])
	})

	t.Run("find ipv4", func(t *testing.T) {
		r := newTestResolver()
		ip, _, err := r.lookup("host4.test.")
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", ip.String())
		assert.True(t, r.ip4["host4.test."])
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
