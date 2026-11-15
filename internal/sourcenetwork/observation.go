package sourcenetwork

import (
	"net"
	"net/netip"
	"net/url"
)

// Observation contains classifications only; never addresses or request data.
// The observer is configured before the client is used and runs in the parent.
type Observation struct {
	Stage   string
	Answers string
	Reason  string
}

func (c *Client) WithObserver(observer func(Observation)) *Client { c.observe = observer; return c }
func (c *Client) observation(stage string, answers []net.IPAddr, reason string) {
	if c.observe != nil {
		c.observe(Observation{Stage: stage, Answers: classifyAnswers(answers), Reason: reason})
	}
}

// Recognizing an IPv6 synthetic range here is diagnostic only. It changes no
// resolver fallback, public-address checks or dialing authority.
var diagnosticFakeV6 = netip.MustParsePrefix("fdfe:dcba:9876::/64")

func deniedURLReason(raw string, media bool) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid_url"
	}
	switch {
	case u.Scheme != "https" && (!media || u.Scheme != "http"):
		return "scheme"
	case u.User != nil:
		return "credentials"
	case u.Fragment != "":
		return "fragment"
	case u.Port() != "" && !(u.Scheme == "https" && u.Port() == "443" || media && u.Scheme == "http" && u.Port() == "80"):
		return "port"
	case u.Hostname() == "":
		return "host"
	}
	return "invalid_url"
}
func classifyAnswers(answers []net.IPAddr) string {
	if len(answers) == 0 {
		return "none"
	}
	result := ""
	for _, answer := range answers {
		kind := "invalid"
		if address, ok := netip.AddrFromSlice(answer.IP); ok {
			address = address.Unmap()
			switch {
			case answer.Zone != "":
				kind = "zoned"
			case fakeIPRange.Contains(address):
				kind = "fake_v4"
			case diagnosticFakeV6.Contains(address):
				kind = "fake_v6"
			case !PublicIP(answer.IP):
				kind = "non_public"
			case address.Is4():
				kind = "public_v4"
			default:
				kind = "public_v6"
			}
		}
		if result == "" {
			result = kind
		} else if result != kind {
			return "mixed"
		}
	}
	return result
}
