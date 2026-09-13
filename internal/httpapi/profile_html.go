package httpapi

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/chrisbirster/vutame/internal/profile"
)

var (
	titlePattern       = regexp.MustCompile(`(?is)<title>.*?</title>`)
	descriptionPattern = regexp.MustCompile(`(?is)<meta\s+name=["']description["'][^>]*>`)
)

type capturedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCapturedResponse() *capturedResponse {
	return &capturedResponse{header: make(http.Header), status: http.StatusOK}
}

func (w *capturedResponse) Header() http.Header { return w.header }
func (w *capturedResponse) WriteHeader(status int) { w.status = status }
func (w *capturedResponse) Write(data []byte) (int, error) { return w.body.Write(data) }

func profileHTMLHandler(web http.Handler, profiles profile.Store, options Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !strings.HasPrefix(r.URL.Path, "/@") {
			web.ServeHTTP(w, r)
			return
		}
		rawHandle := strings.TrimPrefix(r.URL.Path, "/@")
		if rawHandle == "" || strings.Contains(rawHandle, "/") {
			web.ServeHTTP(w, r)
			return
		}
		handle, err := url.PathUnescape(rawHandle)
		if err != nil {
			web.ServeHTTP(w, r)
			return
		}
		item, err := profiles.Get(handle)
		if err != nil {
			web.ServeHTTP(w, r)
			return
		}

		indexRequest := r.Clone(r.Context())
		indexURL := *r.URL
		indexURL.Path = "/"
		indexURL.RawPath = ""
		indexURL.RawQuery = ""
		indexRequest.URL = &indexURL
		indexRequest.Method = http.MethodGet
		captured := newCapturedResponse()
		web.ServeHTTP(captured, indexRequest)
		if captured.status != http.StatusOK || captured.body.Len() == 0 {
			web.ServeHTTP(w, r)
			return
		}

		rendered := renderProfileHTML(captured.body.String(), item, options.ProfileOrigin)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(rendered))
		}
	})
}

func renderProfileHTML(index string, item profile.Profile, profileOrigin string) string {
	origin := strings.TrimRight(strings.TrimSpace(profileOrigin), "/")
	if origin == "" {
		origin = "https://vuta.me"
	}
	canonical := origin + "/@" + url.PathEscape(item.Handle)
	display := strings.TrimSpace(item.DisplayName)
	if display == "" {
		display = "@" + item.Handle
	}
	title := fmt.Sprintf("%s (@%s) — Vutame", display, item.Handle)
	description := strings.TrimSpace(item.Bio)
	if description == "" {
		description = fmt.Sprintf("Links, projects, and places from @%s on Vutame.", item.Handle)
	}
	description = truncateRunes(description, 160)
	image := publicAssetURL(origin, item.AvatarURL)

	index = titlePattern.ReplaceAllString(index, "<title>"+html.EscapeString(title)+"</title>")
	descriptionTag := `<meta name="description" content="` + html.EscapeString(description) + `">`
	if descriptionPattern.MatchString(index) {
		index = descriptionPattern.ReplaceAllString(index, descriptionTag)
	} else {
		index = strings.Replace(index, "</head>", descriptionTag+"\n</head>", 1)
	}

	var meta strings.Builder
	meta.WriteString(`<link rel="canonical" href="` + html.EscapeString(canonical) + `">` + "\n")
	meta.WriteString(`<meta property="og:type" content="profile">` + "\n")
	meta.WriteString(`<meta property="og:title" content="` + html.EscapeString(title) + `">` + "\n")
	meta.WriteString(`<meta property="og:description" content="` + html.EscapeString(description) + `">` + "\n")
	meta.WriteString(`<meta property="og:url" content="` + html.EscapeString(canonical) + `">` + "\n")
	meta.WriteString(`<meta name="twitter:card" content="summary">` + "\n")
	meta.WriteString(`<meta name="twitter:title" content="` + html.EscapeString(title) + `">` + "\n")
	meta.WriteString(`<meta name="twitter:description" content="` + html.EscapeString(description) + `">` + "\n")
	if image != "" {
		meta.WriteString(`<meta property="og:image" content="` + html.EscapeString(image) + `">` + "\n")
		meta.WriteString(`<meta name="twitter:image" content="` + html.EscapeString(image) + `">` + "\n")
	}
	index = strings.Replace(index, "</head>", meta.String()+"</head>", 1)

	noscript := renderNoScriptProfile(item, canonical, image)
	if strings.Contains(index, `<div id="root"></div>`) {
		index = strings.Replace(index, `<div id="root"></div>`, `<noscript>`+noscript+`</noscript><div id="root"></div>`, 1)
	} else {
		index = strings.Replace(index, "</body>", `<noscript>`+noscript+`</noscript></body>`, 1)
	}
	return index
}

func renderNoScriptProfile(item profile.Profile, canonical, image string) string {
	display := strings.TrimSpace(item.DisplayName)
	if display == "" {
		display = "@" + item.Handle
	}
	var output strings.Builder
	output.WriteString(`<main aria-label="Vutame profile">`)
	if image != "" {
		output.WriteString(`<img src="` + html.EscapeString(image) + `" alt="" width="96" height="96">`)
	}
	output.WriteString(`<h1>` + html.EscapeString(display) + `</h1>`)
	output.WriteString(`<p>@` + html.EscapeString(item.Handle) + `</p>`)
	if strings.TrimSpace(item.Bio) != "" {
		output.WriteString(`<p>` + html.EscapeString(item.Bio) + `</p>`)
	}
	if len(item.Links) > 0 {
		output.WriteString(`<ul>`)
		for _, link := range item.Links {
			output.WriteString(`<li><a href="` + html.EscapeString(link.URL) + `">` + html.EscapeString(link.Label) + `</a></li>`)
		}
		output.WriteString(`</ul>`)
	}
	output.WriteString(`<p><a href="` + html.EscapeString(canonical) + `">View this Vuta</a></p>`)
	output.WriteString(`</main>`)
	return output.String()
}

func publicAssetURL(origin, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return parsed.String()
		}
		return ""
	}
	if strings.HasPrefix(value, "/") {
		return strings.TrimRight(origin, "/") + value
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit]))
}
