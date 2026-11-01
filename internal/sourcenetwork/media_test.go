package sourcenetwork

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMediaRedirectsRangesAndNoCredentialForwarding(t *testing.T) {
	c := New()
	calls := 0
	c.media.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Range") != "bytes=10-20" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("range or credential policy changed")
		}
		headers := make(http.Header)
		status := 206
		if calls == 1 {
			status = 302
			headers.Set("Location", "https://cdn.example/next")
		}
		return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader("audio"))}, nil
	})
	response, err := c.OpenMedia(context.Background(), "http://public.example/song", "GET", "bytes=10-20")
	if err != nil || calls != 2 {
		t.Fatal("safe redirects did not work")
	}
	response.Body.Close()
	calls = 0
	c.media.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		headers := make(http.Header)
		headers.Set("Location", "http://127.0.0.1/private")
		return &http.Response{StatusCode: 302, Header: headers, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if _, err := c.OpenMedia(context.Background(), "https://public.example/song", "GET", ""); !errors.Is(err, ErrDenied) || calls != 1 {
		t.Fatal("redirect to LAN was followed")
	}
	for _, value := range []string{"bytes=0-1,100-200", "bytes=0-\r\nsecret", "not-range"} {
		if ValidRange(value) {
			t.Fatal("multi/malformed range accepted")
		}
	}
}
