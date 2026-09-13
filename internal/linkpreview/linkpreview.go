package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxHTMLBytes = 512 << 10

var (
	ErrInvalidURL  = errors.New("invalid preview URL")
	ErrUnavailable = errors.New("preview unavailable")
)

type Metadata struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	Provider    string `json:"provider"`
}

type Fetcher interface {
	Fetch(context.Context, string) (Metadata, error)
}

type Service struct {
	client *http.Client
}

type safeDialer struct {
	resolver *net.Resolver
	dialer   *net.Dialer
}

func NewService() *Service {
	safe := &safeDialer{
		resolver: net.DefaultResolver,
		dialer: &net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           safe.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   2,
		MaxResponseHeaderBytes: 64 << 10,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   6 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return validatePreviewURL(req.URL)
		},
	}
	return &Service{client: client}
}

func (s *Service) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	parsed, err := parsePreviewURL(rawURL)
	if err != nil {
		return Metadata{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9")
	req.Header.Set("User-Agent", "VutameLinkPreview/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: fetch metadata", ErrUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, fmt.Errorf("%w: upstream status %d", ErrUnavailable, resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
		return Metadata{}, fmt.Errorf("%w: upstream is not HTML", ErrUnavailable)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes))
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: read metadata", ErrUnavailable)
	}
	result := parseHTML(body, resp.Request.URL)
	if result.Title == "" && result.Description == "" && result.ImageURL == "" {
		return Metadata{}, fmt.Errorf("%w: no usable metadata", ErrUnavailable)
	}
	return result, nil
}

func parsePreviewURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if len(rawURL) == 0 || len(rawURL) > 2048 {
		return nil, ErrInvalidURL
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	if err := validatePreviewURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validatePreviewURL(parsed *url.URL) error {
	if parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return ErrInvalidURL
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return ErrInvalidURL
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return ErrInvalidURL
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicIP(ip) {
		return ErrInvalidURL
	}
	return nil
}

func (d *safeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, ErrInvalidURL
		}
		return d.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	addresses, err := d.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve preview host: %w", err)
	}
	for _, address := range addresses {
		if !isPublicIP(address.AsSlice()) {
			return nil, ErrInvalidURL
		}
	}
	var lastErr error
	for _, address := range addresses {
		conn, err := d.dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
}

var attributePattern = regexp.MustCompile("(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\\s*=\\s*(?:\"([^\"]*)\"|'([^']*)'|([^\\s\"'=<>`]+))")

func parseHTML(body []byte, base *url.URL) Metadata {
	source := string(body)
	meta := make(map[string]string)
	for _, tag := range metaTags(source) {
		attrs := parseAttributes(tag)
		key := strings.ToLower(strings.TrimSpace(firstNonEmpty(attrs["property"], attrs["name"], attrs["itemprop"])))
		content := cleanText(attrs["content"], 400)
		if key != "" && content != "" {
			if _, exists := meta[key]; !exists {
				meta[key] = content
			}
		}
	}

	title := cleanText(firstNonEmpty(meta["og:title"], meta["twitter:title"], titleText(source)), 160)
	description := cleanText(firstNonEmpty(meta["og:description"], meta["twitter:description"], meta["description"]), 300)
	siteName := cleanText(meta["og:site_name"], 80)
	imageURL := resolveImageURL(firstNonEmpty(meta["og:image:secure_url"], meta["og:image"], meta["twitter:image"]), base)
	provider := providerForHost(base.Hostname())
	if title == "" {
		title = siteName
	}
	return Metadata{
		URL:         base.String(),
		Title:       title,
		Description: description,
		ImageURL:    imageURL,
		SiteName:    siteName,
		Provider:    provider,
	}
}

func metaTags(source string) []string {
	lower := strings.ToLower(source)
	var tags []string
	for offset := 0; ; {
		rel := strings.Index(lower[offset:], "<meta")
		if rel < 0 {
			break
		}
		start := offset + rel
		end := tagEnd(source, start)
		if end < 0 {
			break
		}
		tags = append(tags, source[start:end+1])
		offset = end + 1
	}
	return tags
}

func tagEnd(source string, start int) int {
	var quote byte
	for index := start; index < len(source); index++ {
		char := source[index]
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == '>' {
			return index
		}
	}
	return -1
}

func parseAttributes(tag string) map[string]string {
	result := make(map[string]string)
	for _, match := range attributePattern.FindAllStringSubmatch(tag, -1) {
		value := firstNonEmpty(match[2], match[3], match[4])
		result[strings.ToLower(match[1])] = html.UnescapeString(value)
	}
	return result
}

func titleText(source string) string {
	lower := strings.ToLower(source)
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	openEnd := tagEnd(source, start)
	if openEnd < 0 {
		return ""
	}
	closeStart := strings.Index(lower[openEnd+1:], "</title>")
	if closeStart < 0 {
		return ""
	}
	return html.UnescapeString(source[openEnd+1 : openEnd+1+closeStart])
}

func resolveImageURL(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "https" || resolved.Hostname() == "" || resolved.User != nil {
		return ""
	}
	return resolved.String()
}

func providerForHost(host string) string {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(host, ".")), "www.")
	switch {
	case host == "github.com":
		return "github"
	case host == "youtu.be" || host == "youtube.com" || strings.HasSuffix(host, ".youtube.com"):
		return "youtube"
	case host == "instagram.com" || strings.HasSuffix(host, ".instagram.com"):
		return "instagram"
	case host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com"):
		return "tiktok"
	case host == "x.com" || strings.HasSuffix(host, ".x.com") || host == "twitter.com" || strings.HasSuffix(host, ".twitter.com"):
		return "x"
	case host == "bsky.app" || strings.HasSuffix(host, ".bsky.app"):
		return "bluesky"
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		return "linkedin"
	case host == "spotify.com" || strings.HasSuffix(host, ".spotify.com"):
		return "spotify"
	default:
		return "website"
	}
}

func cleanText(value string, maxRunes int) string {
	value = html.UnescapeString(value)
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
