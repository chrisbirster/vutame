package httpapi

import (
	"net"
	"net/http"
	"strings"
)

func remoteRateIdentity(r *http.Request) string {
	value := strings.TrimSpace(r.RemoteAddr)
	if value == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(value); err == nil && host != "" {
		return host
	}
	return value
}
