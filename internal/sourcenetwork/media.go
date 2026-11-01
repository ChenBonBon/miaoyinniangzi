package sourcenetwork

import (
	"context"
	"net"
	"net/http"
	"regexp"
)

var singleRange = regexp.MustCompile(`^bytes=(?:[0-9]{1,12}-[0-9]{0,12}|-[0-9]{1,12})$`)

func ValidRange(value string) bool {
	return value == "" || len(value) <= 40 && singleRange.MatchString(value)
}
func (c *Client) dialPinned(ctx context.Context, address string, media bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || (port != "443" && (!media || port != "80")) {
		return nil, ErrDenied
	}
	ips, err := c.addresses(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		connection, err := c.dial(ctx, "tcp", net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, ErrNetwork
}

// OpenMedia streams without buffering the song. Each redirect and each new
// connection gets the same DNS/IP policy; no script headers or API credentials
// are forwarded. The caller must bound bytes, duration and close the response.
func (c *Client) OpenMedia(ctx context.Context, raw, method, byteRange string) (*http.Response, error) {
	if method != "GET" && method != "HEAD" || !ValidRange(byteRange) {
		return nil, ErrDenied
	}
	// No whole-body Client.Timeout: playback has a separate bounded lifetime.
	client := c.media
	for redirects := 0; redirects <= 3; redirects++ {
		u, err := validEndpointURL(raw, true)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
		if err != nil {
			return nil, ErrDenied
		}
		request.Header.Set("Accept-Encoding", "identity")
		request.Header.Set("User-Agent", "Miaoyin/0.1 (audio)")
		if byteRange != "" {
			request.Header.Set("Range", byteRange)
		}
		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ErrNetwork
			}
			return nil, ErrNetwork
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			next, err := response.Location()
			response.Body.Close()
			if err != nil {
				return nil, ErrDenied
			}
			raw = next.String()
			continue
		}
		if response.StatusCode != 200 && response.StatusCode != 206 && response.StatusCode != 416 {
			response.Body.Close()
			return nil, ErrNetwork
		}
		return response, nil
	}
	return nil, ErrDenied
}
