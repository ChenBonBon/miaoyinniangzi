package sourcenetwork

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func TestLiveMixedDNSCompatibility(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_SOURCE_DNS") != "1" {
		t.Skip("explicit public DNS opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c := New().WithObserver(func(event Observation) {
		t.Logf("stage=%s answers=%s reason=%s", event.Stage, event.Answers, event.Reason)
	})
	addresses, err := c.addresses(ctx, "u.y.qq.com")
	if err != nil || len(addresses) == 0 {
		t.Fatal("public DNS compatibility failed")
	}
	for _, address := range addresses {
		if !PublicIP(address) {
			t.Fatal("unsafe result")
		}
	}
	t.Logf("validated public address count=%d; no source or media request", len(addresses))
}

func TestMixedSyntheticAnswersAreReplacedNotTrusted(t *testing.T) {
	for _, other := range []string{"fd00:1234::1", "fdfe:dcba:9876::1", "127.0.0.1", "9.9.9.9"} {
		t.Run(other, func(t *testing.T) {
			c := New()
			calls := 0
			dials := 0
			c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("198.18.0.2")}, {IP: net.ParseIP(other)}}, nil
			}
			c.localAddrs = func() ([]net.Addr, error) { return nil, nil }
			c.fallback = func(_ context.Context, host string) ([]net.IPAddr, error) {
				calls++
				if host != "api.example" {
					t.Fatal("wrong query")
				}
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			}
			c.dial = func(_ context.Context, _, address string) (net.Conn, error) {
				dials++
				if address != "8.8.8.8:443" {
					t.Fatal("original answer reached dial")
				}
				return nil, ErrNetwork
			}
			_, err := c.dialPinned(context.Background(), "api.example:443", false)
			if !errors.Is(err, ErrNetwork) || calls != 1 || dials != 1 {
				t.Fatal("fresh pinned lookup missing")
			}
			for _, unsafe := range []string{"127.0.0.1", "fd00::1", "198.18.0.3"} {
				c.fallback = func(context.Context, string) ([]net.IPAddr, error) {
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP(unsafe)}}, nil
				}
				dials = 0
				_, err = c.dialPinned(context.Background(), "api.example:443", false)
				if !errors.Is(err, ErrDenied) || dials != 0 {
					t.Fatal("unsafe fallback allowed a connection")
				}
			}
			c.fallback = func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			}
			c.localAddrs = func() ([]net.Addr, error) {
				return []net.Addr{&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(32, 32)}}, nil
			}
			if _, err = c.addresses(context.Background(), "api.example"); !errors.Is(err, ErrDenied) {
				t.Fatal("local interface allowed")
			}
		})
	}
}
func TestPrivateMixedAnswersWithoutSyntheticMarkerStayDenied(t *testing.T) {
	c := New()
	c.localAddrs = func() ([]net.Addr, error) { return nil, nil }
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("fd00::1")}}, nil
	}
	c.fallback = func(context.Context, string) ([]net.IPAddr, error) {
		t.Fatal("ordinary private mixture reinterpreted as synthetic")
		return nil, nil
	}
	if _, err := c.addresses(context.Background(), "api.example"); !errors.Is(err, ErrDenied) {
		t.Fatal("private mixture allowed")
	}
}
