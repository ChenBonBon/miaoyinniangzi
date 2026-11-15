package sourcenetwork

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

var fakeIPRange = netip.MustParsePrefix("198.18.0.0/15")

// A known synthetic IPv4 answer triggers a fresh, independently validated
// lookup of the hostname, never permission to use any original answer. TUN
// resolvers may mix that IPv4 with synthetic/private IPv6. The entire original
// set must be discarded, including apparently public addresses. Without a
// known synthetic IPv4 marker the ordinary all-address rejection is unchanged.
func needsPublicDNS(answers []net.IPAddr) bool {
	found := false
	for _, answer := range answers {
		ip, ok := netip.AddrFromSlice(answer.IP)
		if !ok || answer.Zone != "" {
			return false
		}
		if fakeIPRange.Contains(ip.Unmap()) {
			found = true
		}
	}
	return found
}

// A TUN's synthetic addresses are never dialed. Bootstrap one fixed HTTPS DNS
// authority by its public IP, then apply the normal public/local-interface rules
// to the real answers. Only the hostname, never paths/headers/keys, is sent.
var publicDNSHTTP = func() *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: 2 * time.Second, MaxResponseHeaderBytes: 16 * 1024, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", "223.5.5.5:443")
	}}
	return &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}()

func publicDNS(ctx context.Context, host string) ([]net.IPAddr, error) {
	if len(host) == 0 || len(host) > 253 {
		return nil, ErrDenied
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://dns.alidns.com/resolve?"+url.Values{"name": {host}, "type": {"A"}}.Encode(), nil)
	if err != nil {
		return nil, ErrNetwork
	}
	request.Header.Set("Accept", "application/dns-json")
	response, err := publicDNSHTTP.Do(request)
	if err != nil {
		return nil, ErrNetwork
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, ErrNetwork
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16*1024+1))
	if err != nil || len(body) > 16*1024 {
		return nil, ErrNetwork
	}
	return decodePublicDNS(body)
}
func decodePublicDNS(body []byte) ([]net.IPAddr, error) {
	var response struct {
		Status *int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.Unmarshal(body, &response) != nil || response.Status == nil || *response.Status != 0 || len(response.Answer) > 64 {
		return nil, ErrNetwork
	}
	result := []net.IPAddr{}
	for _, answer := range response.Answer {
		if answer.Type == 1 {
			ip := net.ParseIP(answer.Data)
			if ip == nil || ip.To4() == nil || !PublicIP(ip) {
				return nil, ErrDenied
			}
			result = append(result, net.IPAddr{IP: ip})
		}
	}
	if len(result) == 0 || len(result) > 32 {
		return nil, ErrNetwork
	}
	return result, nil
}
