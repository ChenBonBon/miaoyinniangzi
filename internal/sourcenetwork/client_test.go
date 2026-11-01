package sourcenetwork

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPublicNetworkOnly(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.0.1", "172.16.0.1", "100.100.100.200", "169.254.169.254", "::ffff:127.0.0.1", "0.0.0.0", "224.1.2.3", "240.1.2.3", "198.18.0.1", "168.63.129.16", "fc00::1", "fe80::1", "64:ff9b::a00:1", "2002:a00:1::1", "2001:db8::1", "2001:20::1", "fec0::1"} {
		if PublicIP(net.ParseIP(raw)) {
			t.Errorf("allowed %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"} {
		if !PublicIP(net.ParseIP(raw)) {
			t.Errorf("blocked public %s", raw)
		}
	}
	for _, raw := range []string{"http://example.com/a", "file:///etc/passwd", "https://user:key@example.com/a", "https://example.com:8443/a", "https://127.0.0.1/a", "https://[::1]/a", "https://example.com/a#fragment", "https://example.com/\r\n"} {
		if _, err := validURL(raw); err == nil {
			t.Errorf("allowed unsafe URL")
		}
	}
}
func TestMixedDNSAndRebindingCannotReachLAN(t *testing.T) {
	c := New()
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, err := c.addresses(context.Background(), "public.example"); !errors.Is(err, ErrDenied) {
		t.Fatal("mixed answers accepted")
	}
	transport := c.http.Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "public.example:443"); !errors.Is(err, ErrDenied) {
		t.Fatal("dial bypassed DNS validation")
	}
	if err := c.ValidateMedia(context.Background(), "https://public.example/a"); err == nil {
		t.Fatal("unsafe media accepted")
	}
	if transport.Proxy != nil || c.http.Jar != nil {
		t.Fatal("inherited credentials/proxy")
	}
}
func TestDialPinsValidatedAddressWithoutSecondLookup(t *testing.T) {
	c := New()
	lookups := 0
	dials := 0
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		if lookups > 1 {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	c.dial = func(_ context.Context, network, address string) (net.Conn, error) {
		dials++
		if network != "tcp" || address != "8.8.8.8:443" {
			t.Fatal("validated IP was not pinned")
		}
		return nil, errors.New("fixture: no actual network")
	}
	_, _ = c.http.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "rebind.example:443")
	if lookups != 1 || dials != 1 {
		t.Fatal("unexpected DNS/dial behavior")
	}
}

func TestFakeIPRequiresRealPublicDNSAnswers(t *testing.T) {
	c := New()
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.10")}}, nil
	}
	c.localAddrs = func() ([]net.Addr, error) { return nil, nil }
	calls := 0
	c.fallback = func(_ context.Context, host string) ([]net.IPAddr, error) {
		calls++
		if host != "api.example" {
			t.Fatal("wrong DNS question")
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	ips, err := c.addresses(context.Background(), "api.example")
	if err != nil || len(ips) != 1 || ips[0].String() != "8.8.8.8" || calls != 1 {
		t.Fatal("fake IP was not replaced")
	}
	c.fallback = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, err := c.addresses(context.Background(), "api.example"); !errors.Is(err, ErrDenied) {
		t.Fatal("DNS fallback bypassed private IP checks")
	}
	if _, err := validURL("https://198.18.0.10/a"); err == nil {
		t.Fatal("literal benchmark IP was allowed")
	}
	if _, err := decodePublicDNS([]byte(`{"Status":0,"Answer":[{"type":1,"data":"127.0.0.1"}]}`)); !errors.Is(err, ErrDenied) {
		t.Fatal("private DoH answer accepted")
	}
	if _, err := decodePublicDNS([]byte(`{"Status":0,"Answer":[{"type":1,"data":"8.8.8.8"}]}`)); err != nil {
		t.Fatal("public DoH answer rejected")
	}
}

func TestMediaHTTPAndHTTPSAreSeparateFromSourceAPIPolicy(t *testing.T) {
	c := New()
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	c.localAddrs = func() ([]net.Addr, error) { return nil, nil }
	for _, raw := range []string{"http://cdn.example/song.flac", "http://cdn.example:80/song.flac", "https://cdn.example/song.flac", "https://cdn.example:443/song.flac"} {
		if err := c.ValidateMedia(context.Background(), raw); err != nil {
			t.Fatalf("normal media URL blocked: %v", err)
		}
	}
	for _, raw := range []string{"http://127.0.0.1/a", "http://169.254.169.254/a", "http://cdn.example:443/a", "https://cdn.example:80/a", "http://user:key@cdn.example/a", "file:///etc/passwd", "http://cdn.example/a#fragment"} {
		if err := c.ValidateMedia(context.Background(), raw); err == nil {
			t.Fatal("unsafe media accepted")
		}
	}
	if _, err := validURL("http://api.example/resolve"); err == nil {
		t.Fatal("source API HTTPS requirement was relaxed")
	}
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if err := c.ValidateMedia(context.Background(), "http://cdn.example/song"); err == nil {
		t.Fatal("HTTP media bypassed DNS checks")
	}
}

func TestOwnPublicInterfaceIsNotAnAllowedRemote(t *testing.T) {
	c := New()
	c.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	c.localAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(32, 32)}}, nil
	}
	if _, err := c.addresses(context.Background(), "own.example"); !errors.Is(err, ErrDenied) {
		t.Fatal("own public interface accepted")
	}
}
func TestLivePublicHTTPSProxy(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_SOURCE_PROXY") != "1" {
		t.Skip("public HTTPS opt-in")
	}
	response, err := New().Do(context.Background(), Request{URL: "https://c.y.qq.com/splcloud/fcgi-bin/smartbox_new.fcg?key=%E5%8F%AA%E6%83%B3&format=json", Method: "GET"})
	if err != nil || response.Status != 200 || len(response.Body) == 0 {
		t.Fatalf("public HTTPS request failed: %v (status %d)", err, response.Status)
	}
	t.Log("validated-IP HTTPS transport passed; no source service used")
}

func TestProxyBoundsAndCredentialRules(t *testing.T) {
	c := New()
	calls := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie leaked")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":200,"url":"https://cdn.example/a"}`)), Header: make(http.Header)}, nil
	})
	for _, request := range []Request{{URL: "https://example.com", Method: "DELETE"}, {URL: "https://example.com", Method: "POST", Body: make([]byte, 65537)}, {URL: "https://example.com", Method: "GET", Headers: map[string]string{"Host": "localhost"}}, {URL: "https://example.com", Method: "GET", Headers: map[string]string{"Cookie": "secret"}}, {URL: "https://example.com", Method: "GET", Headers: map[string]string{"X-Key": "a\r\nb"}}} {
		if _, err := c.Do(context.Background(), request); err == nil {
			t.Fatal("unsafe request accepted")
		}
	}
	if calls != 0 {
		t.Fatal("unsafe input reached transport")
	}
	if _, err := c.Do(context.Background(), Request{URL: "https://example.com", Method: "POST", Headers: map[string]string{"Authorization": "Bearer user-source-key"}, Body: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		status int
		body   string
		want   error
	}{{403, "secret", ErrRestricted}, {429, "secret", ErrRestricted}, {302, "secret", ErrDenied}, {500, "secret", ErrNetwork}, {200, `{"code":403}`, ErrRestricted}, {200, strings.Repeat("x", MaxBody+1), ErrNetwork}} {
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header)}, nil
		})
		if _, err := c.Do(context.Background(), Request{URL: "https://example.com", Method: "GET"}); !errors.Is(err, test.want) {
			t.Fatalf("status %d: %v", test.status, err)
		}
	}
}
