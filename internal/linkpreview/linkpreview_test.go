package linkpreview

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseHTMLPrefersOpenGraphAndInfersProvider(t *testing.T) {
	base, err := url.Parse("https://github.com/example/project")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`<!doctype html><html><head>
		<title>Fallback title</title>
		<meta content="Example &amp; Project" property="og:title">
		<meta name="description" content="Fallback description">
		<meta property="og:description" content="A &lt;safe&gt; project description">
		<meta property="og:image" content="/social/card.png">
		<meta property="og:site_name" content="GitHub">
	</head></html>`)

	metadata := parseHTML(body, base)
	if metadata.Title != "Example & Project" {
		t.Fatalf("title = %q", metadata.Title)
	}
	if metadata.Description != "A <safe> project description" {
		t.Fatalf("description = %q", metadata.Description)
	}
	if metadata.ImageURL != "https://github.com/social/card.png" {
		t.Fatalf("image = %q", metadata.ImageURL)
	}
	if metadata.Provider != "github" || metadata.SiteName != "GitHub" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestParseHTMLFallsBackToTitleAndWebsiteProvider(t *testing.T) {
	base, _ := url.Parse("https://example.com/article")
	metadata := parseHTML([]byte(`<html><head><title>  Plain   title </title></head></html>`), base)
	if metadata.Title != "Plain title" || metadata.Provider != "website" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestCleanTextIsBounded(t *testing.T) {
	value := cleanText(strings.Repeat("🙂", 200), 12)
	if len([]rune(value)) != 12 {
		t.Fatalf("rune count = %d", len([]rune(value)))
	}
}
