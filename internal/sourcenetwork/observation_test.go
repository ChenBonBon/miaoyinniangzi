package sourcenetwork

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestNetworkObservationsClassifyWithoutChangingAuthority(t *testing.T) {
	for _, tc := range []struct{ ip, kind string }{{"198.18.0.2", "fake_v4"}, {"fdfe:dcba:9876::1", "fake_v6"}, {"10.0.0.2", "non_public"}, {"8.8.8.8", "public_v4"}, {"2606:4700:4700::1111", "public_v6"}} {
		if got := classifyAnswers([]net.IPAddr{{IP: net.ParseIP(tc.ip)}}); got != tc.kind {
			t.Fatal("wrong classification")
		}
	}
	if classifyAnswers([]net.IPAddr{{IP: net.ParseIP("198.18.0.2")}, {IP: net.ParseIP("fdfe:dcba:9876::1")}}) != "mixed" {
		t.Fatal("mixed hidden")
	}
	var events []Observation
	client := New().WithObserver(func(event Observation) { events = append(events, event) })
	client.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("fdfe:dcba:9876::123")}}, nil
	}
	client.localAddrs = func() ([]net.Addr, error) { return nil, nil }
	client.fallback = func(context.Context, string) ([]net.IPAddr, error) {
		t.Fatal("diagnostic recognition widened fallback")
		return nil, nil
	}
	if _, err := client.addresses(context.Background(), "PRIVATE-HOST.invalid"); !errors.Is(err, ErrDenied) {
		t.Fatal("private IPv6 must remain denied")
	}
	if len(events) != 3 || events[0].Answers != "fake_v6" || events[2].Reason != "non_public_or_zone" {
		t.Fatal("denial stage missing")
	}
	_, _ = client.Do(context.Background(), Request{URL: "http://PRIVATE-HOST.invalid/SECRET?key=SECRET", Method: "GET"})
	if events[len(events)-1].Reason != "scheme" {
		t.Fatal("scheme denial hidden")
	}
	encoded, _ := json.Marshal(events)
	for _, secret := range []string{"PRIVATE-HOST", "SECRET", "fdfe:", "https://", "http://"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("observation disclosed request data")
		}
	}
}
