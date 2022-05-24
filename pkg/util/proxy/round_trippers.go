package proxy

import (
	"net/http"
	"strings"

	"k8s.io/client-go/transport"
)

type ProxyHeaderRoundTripper struct {
	proxyHeaders http.Header
	roundTripper http.RoundTripper
}

// NewProxyHeaderRoundTripperWrapperConstructor returns a RoundTripper wrapper that's usable within restConfig.WrapTransport.
func NewProxyHeaderRoundTripperWrapperConstructor(wt transport.WrapperFunc, headers map[string]string) transport.WrapperFunc {
	return func(rt http.RoundTripper) http.RoundTripper {
		if wt != nil {
			rt = wt(rt)
		}
		return &ProxyHeaderRoundTripper{
			proxyHeaders: ParseProxyHeaders(headers),
			roundTripper: rt,
		}
	}
}

// RoundTrip implements the http.RoundTripper interface
func (r *ProxyHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if tr, ok := r.roundTripper.(*http.Transport); ok {
		tr.ProxyConnectHeader = r.proxyHeaders
		return tr.RoundTrip(req)
	}
	return r.roundTripper.RoundTrip(req)
}

// ParseProxyHeaders will parse headers to send to proxies from given map.
func ParseProxyHeaders(headers map[string]string) http.Header {
	if len(headers) == 0 {
		return nil
	}

	proxyHeaders := make(http.Header)
	for key, values := range headers {
		proxyHeaders[key] = strings.Split(values, ",")
	}
	return proxyHeaders
}
