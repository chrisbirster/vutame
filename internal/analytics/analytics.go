package analytics

import (
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
)

type Metadata struct {
	ReferrerHost string
	DeviceClass  string
	Bot          bool
}

func MetadataFromHeaders(referrer, userAgent string) Metadata {
	return Metadata{
		ReferrerHost: referrerHost(referrer),
		DeviceClass:  deviceClass(userAgent),
		Bot:          isBot(userAgent),
	}
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
