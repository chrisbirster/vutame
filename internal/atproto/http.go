package atproto

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const maxATProtoResponse = 2 << 20

func hardenedClient(allowHTTP bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if allowHTTP && (host == "127.0.0.1" || host == "::1" || host == "localhost") {
				return dialer.DialContext(ctx, network, address)
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, fmt.Errorf("resolve AT Protocol host")
			}
			for _, candidate := range addresses {
				if publicATAddress(candidate) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
				}
			}
			return nil, fmt.Errorf("AT Protocol host does not resolve to a public address")
		},
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many AT Protocol redirects")
		}
		return validateATURL(req.URL.String(), allowHTTP)
	}
	return client
}

func publicATAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsGlobalUnicast() && !address.IsLoopback() && !address.IsPrivate() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() && !address.IsUnspecified()
}

func validateATURL(value string, allowHTTP bool) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidIdentity
	}
	if parsed.Scheme != "https" {
		if !(allowHTTP && parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1" || parsed.Hostname() == "localhost")) {
			return ErrInvalidIdentity
		}
	}
	if parsed.Port() != "" && !(allowHTTP && parsed.Scheme == "http") && parsed.Port() != "443" {
		return ErrInvalidIdentity
	}
	if len(parsed.String()) > 4096 {
		return ErrInvalidIdentity
	}
	if address, err := netip.ParseAddr(parsed.Hostname()); err == nil && !publicATAddress(address) {
		if !(allowHTTP && address.IsLoopback()) {
			return ErrInvalidIdentity
		}
	}
	return nil
}

func readJSONResponse(response *http.Response, target any) error {
	defer response.Body.Close()
	reader := io.LimitReader(response.Body, maxATProtoResponse+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(data) > maxATProtoResponse {
		return errors.New("AT Protocol response too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var apiError struct { Error string `json:"error"`; Message string `json:"message"` }
		_ = json.Unmarshal(data, &apiError)
		message := strings.TrimSpace(apiError.Message)
		if message == "" { message = strings.TrimSpace(apiError.Error) }
		if message == "" { message = response.Status }
		return fmt.Errorf("AT Protocol request failed: %s", message)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode AT Protocol response: %w", err)
	}
	return nil
}
