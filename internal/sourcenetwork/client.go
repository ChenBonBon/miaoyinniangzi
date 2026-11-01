// Package sourcenetwork is the only HTTP authority available to source workers.
package sourcenetwork

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrDenied = errors.New("音源请求被安全策略拒绝")
var ErrNetwork = errors.New("音源网络请求失败")
var ErrRestricted = errors.New("音源服务拒绝请求或请求过于频繁")

const MaxBody = 1024 * 1024

type Request struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}
type Response struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}
type Proxy interface {
	Do(context.Context, Request) (Response, error)
	ValidateMedia(context.Context, string) error
}
type Client struct {
	http       *http.Client
	media      *http.Client
	lookup     func(context.Context, string) ([]net.IPAddr, error)
	dial       func(context.Context, string, string) (net.Conn, error)
	localAddrs func() ([]net.Addr, error)
	fallback   func(context.Context, string) ([]net.IPAddr, error)
}

func New() *Client {
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	c := &Client{lookup: net.DefaultResolver.LookupIPAddr, dial: dialer.DialContext, localAddrs: net.InterfaceAddrs, fallback: publicDNS}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 6 * time.Second, MaxResponseHeaderBytes: 32 * 1024, DisableKeepAlives: true}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return c.dialPinned(ctx, address, false)
	}
	c.http = &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	mediaTransport := transport.Clone()
	mediaTransport.DialContext = func(ctx context.Context, _, address string) (net.Conn, error) {
		return c.dialPinned(ctx, address, true)
	}
	c.media = &http.Client{Transport: mediaTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "168.63.129.16/32", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:db8::/32", "2001:2::/48", "2001:10::/28", "2001:20::/28", "3fff::/20", "fec0::/10", "2002::/16", "fc00::/7", "fe80::/10"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

func PublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
		return false
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
func validURL(raw string) (*url.URL, error) { return validEndpointURL(raw, false) }
func validEndpointURL(raw string, media bool) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 8192 || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, ErrDenied
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Hostname() == "" {
		return nil, ErrDenied
	}
	port := "443"
	if media && u.Scheme == "http" {
		port = "80"
	} else if u.Scheme != "https" {
		return nil, ErrDenied
	}
	if u.Port() != "" && u.Port() != port {
		return nil, ErrDenied
	}
	host := u.Hostname()
	if len(host) > 253 {
		return nil, ErrDenied
	}
	for _, r := range host {
		if r > 127 || r <= 32 || r == '%' {
			return nil, ErrDenied
		}
	}
	if ip := net.ParseIP(host); ip != nil && !PublicIP(ip) {
		return nil, ErrDenied
	}
	return u, nil
}
func (c *Client) addresses(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := c.lookup(ctx, host)
	if err != nil || len(ips) == 0 || len(ips) > 32 {
		return nil, ErrNetwork
	}
	if fakeIPAnswers(ips) {
		ips, err = c.fallback(ctx, host)
		if err != nil || len(ips) == 0 || len(ips) > 32 {
			if errors.Is(err, ErrDenied) {
				return nil, ErrDenied
			}
			return nil, ErrNetwork
		}
	}
	local, err := c.localAddrs()
	if err != nil {
		return nil, ErrDenied
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		for _, address := range local {
			var localIP net.IP
			switch v := address.(type) {
			case *net.IPNet:
				localIP = v.IP
			case *net.IPAddr:
				localIP = v.IP
			}
			if localIP != nil && localIP.Equal(ip.IP) {
				return nil, ErrDenied
			}
		}
		if ip.Zone != "" || !PublicIP(ip.IP) {
			return nil, ErrDenied
		}
		out = append(out, ip.IP)
	}
	return out, nil
}
func (c *Client) ValidateMedia(ctx context.Context, raw string) error {
	u, err := validEndpointURL(raw, true)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = c.addresses(ctx, u.Hostname())
	return err
}
func (c *Client) Do(ctx context.Context, input Request) (Response, error) {
	u, err := validURL(input.URL)
	if err != nil {
		return Response{}, err
	}
	if input.Method != "GET" && input.Method != "POST" || len(input.Body) > 64*1024 || len(input.Headers) > 32 {
		return Response{}, ErrDenied
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, u.String(), bytes.NewReader(input.Body))
	if err != nil {
		return Response{}, ErrDenied
	}
	total := 0
	for key, value := range input.Headers {
		total += len(key) + len(value)
		switch strings.ToLower(key) {
		case "host", "cookie", "proxy-authorization", "proxy-connection", "connection", "transfer-encoding", "content-length", "accept-encoding", "upgrade", "trailer":
			return Response{}, ErrDenied
		}
		if len(key) > 128 || len(value) > 4096 || total > 16*1024 || strings.ContainsAny(key+value, "\r\n\x00") {
			return Response{}, ErrDenied
		}
		request.Header.Set(key, value)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if errors.Is(err, ErrDenied) {
			return Response{}, ErrDenied
		}
		return Response{}, ErrNetwork
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 429 {
		return Response{}, ErrRestricted
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return Response{}, ErrDenied
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, ErrNetwork
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxBody+1))
	if err != nil || len(body) > MaxBody {
		return Response{}, ErrNetwork
	}
	// Common source services put authorization/rate-limit errors in JSON even
	// with HTTP 200. Stop rather than retrying those through lower qualities.
	var envelope struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(body, &envelope) == nil && (envelope.Code == 401 || envelope.Code == 403 || envelope.Code == 429) {
		return Response{}, ErrRestricted
	}
	return Response{Status: response.StatusCode, Body: body}, nil
}
