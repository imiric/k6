package netext

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func makeTestResolver() (*resolver, context.CancelFunc) {
	r := NewResolver().(*resolver)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	r.ctx = ctx
	return r, cancel
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	detectInterfaces() // Reset network interfaces config
	os.Exit(exitCode)
}

func TestLookup(t *testing.T) {
	t.Run("never resolved", func(t *testing.T) {
		ip6 = true
		ip4 = true
		r, _ := makeTestResolver()
		require.False(t, r.ip4["example.com."])
	})

	t.Run("resolution failure", func(t *testing.T) {
		ip6 = true
		ip4 = true
		r, cancel := makeTestResolver()
		defer cancel()
		_, _, err := r.lookup("example.badtld.")
		require.Error(t, err)
		require.False(t, r.ip4["example.badtld."])
	})

	t.Run("find ipv6", func(t *testing.T) {
		ip6 = true
		ip4 = false
		r, cancel := makeTestResolver()
		defer cancel()
		ip, _, err := r.lookup("example.com.")
		require.NoError(t, err)
		require.True(t, ip.To4() == nil)
		require.False(t, r.ip4["example.com."])
	})

	t.Run("find ipv4", func(t *testing.T) {
		ip6 = true
		ip4 = true
		r, cancel := makeTestResolver()
		defer cancel()
		ip, _, err := r.lookup("httpbin.org.")
		require.NoError(t, err)
		require.True(t, ip.To4() != nil)
		require.True(t, r.ip4["httpbin.org."])
	})
}

func TestResolution(t *testing.T) {
	t.Run("follow CNAMEs", func(t *testing.T) {
		ip6 = true
		ip4 = true
		r, cancel := makeTestResolver()
		defer cancel()
		ip, err := r.Resolve(r.ctx, "http2.akamai.com", 3)
		require.NoError(t, err)
		require.True(t, ip.To4() != nil)
		cname := r.cname["http2.akamai.com."]
		require.NotEqual(t, cname, nil)
		require.Equal(t, "http2.edgekey.net.", cname.Name)
	})
}
