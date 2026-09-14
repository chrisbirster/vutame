package analytics

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
)

const (
	KindProfileView = "profile_view"
	KindLinkClick   = "link_click"

	DeviceDesktop = "desktop"
	DeviceMobile  = "mobile"
	DeviceTablet  = "tablet"
	DeviceUnknown = "unknown"

	DefaultDashboardDays = 30
	MaxDashboardDays     = 90
	MaxCampaignLength    = 80
)

type Metadata struct {
	VisitorHash  string
	Campaign     string
	ReferrerHost string
	DeviceClass  string
	Bot          bool
}

type Summary struct {
	ProfileViews   int `json:"profile_views"`
	LinkClicks     int `json:"link_clicks"`
	UniqueVisitors int `json:"unique_visitors"`
}

type DailyPoint struct {
	Date           string `json:"date"`
	ProfileViews   int    `json:"profile_views"`
	LinkClicks     int    `json:"link_clicks"`
	UniqueVisitors int    `json:"unique_visitors"`
}

type LinkMetric struct {
	LinkID string `json:"link_id"`
	Label  string `json:"label"`
	Clicks int    `json:"clicks"`
}

type Breakdown struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Dashboard struct {
	Days      int          `json:"days"`
	StartDate string       `json:"start_date"`
	EndDate   string       `json:"end_date"`
	Summary   Summary      `json:"summary"`
	Series    []DailyPoint `json:"series"`
	TopLinks  []LinkMetric `json:"top_links"`
	Referrers []Breakdown  `json:"referrers"`
	Devices   []Breakdown  `json:"devices"`
	Campaigns []Breakdown  `json:"campaigns"`
}

func MetadataFromHeaders(referrer, userAgent string) Metadata {
	return Metadata{
		ReferrerHost: referrerHost(referrer),
		DeviceClass:  deviceClass(userAgent),
		Bot:          isBot(userAgent),
	}
}

func NormalizeCampaign(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len([]rune(value)) > MaxCampaignLength {
		return ""
	}
	return value
}

func NormalizeDashboardDays(days int) int {
	if days <= 0 {
		return DefaultDashboardDays
	}
	if days > MaxDashboardDays {
		return MaxDashboardDays
	}
	return days
}

// ClientIP accepts Fly.io's platform-provided client IP when it is a valid
// address, otherwise it falls back to the socket remote address. Generic
// forwarding headers are deliberately ignored because clients can spoof them.
func ClientIP(flyClientIP, remoteAddr string) string {
	if address, err := netip.ParseAddr(strings.TrimSpace(flyClientIP)); err == nil {
		return address.Unmap().String()
	}
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	address, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return ""
	}
	return address.Unmap().String()
}

func referrerHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	if len(host) > 253 {
		return ""
	}
	return host
}

func deviceClass(userAgent string) string {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return DeviceUnknown
	}
	if strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") {
		return DeviceTablet
	}
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "iphone") || strings.Contains(ua, "android") {
		return DeviceMobile
	}
	return DeviceDesktop
}

func isBot(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
	}
	for _, marker := range []string{
		"bot", "crawler", "spider", "slurp", "facebookexternalhit", "twitterbot",
		"slackbot", "discordbot", "linkedinbot", "preview", "curl/", "wget/",
	} {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}
