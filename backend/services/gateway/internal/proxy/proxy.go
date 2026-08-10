// Package proxy builds reverse proxies to each backend service that strip the
// /api prefix before forwarding.
package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	apiutil "github.com/aaks/server/internal/httputil"
)

// New returns a reverse proxy to upstream that strips the leading /api from the
// request path. Upstream connection failures become a generic 502 (no internal
// detail leaked).
func New(upstream string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	orig := rp.Director
	rp.Director = func(req *http.Request) {
		orig(req)
		if strings.HasPrefix(req.URL.Path, "/api") {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}
		req.Host = target.Host
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		slog.Error("upstream proxy error", "upstream", upstream, "error", err)
		apiutil.Error(w, http.StatusBadGateway, "upstream unavailable")
	}
	return rp, nil
}
