package capture

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var titleRegex = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)

// WebCapture represents captured content from a URL
type WebCapture struct {
	URL     string
	Title   string
	Content string
	Impetus string
}

// CaptureFromURL fetches a URL and extracts title
func CaptureFromURL(url string, additionalContent string) (*WebCapture, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		// Fallback: just use URL
		return &WebCapture{
			URL:     url,
			Title:   "",
			Content: buildContent(url, "", additionalContent),
			Impetus: inferImpetusFromURL(url),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	title := extractTitle(string(body))

	capture := &WebCapture{
		URL:     url,
		Title:   title,
		Content: buildContent(url, title, additionalContent),
		Impetus: inferImpetusFromURL(url),
	}

	return capture, nil
}

func buildContent(url, title, additionalContent string) string {
	if additionalContent != "" {
		return fmt.Sprintf("%s\n\n%s", additionalContent, url)
	}
	if title != "" {
		return fmt.Sprintf("%s\n\n%s", title, url)
	}
	return url
}

func extractTitle(html string) string {
	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		// Clean up common suffixes
		if idx := strings.Index(title, " | "); idx > 0 {
			title = title[:idx]
		}
		if idx := strings.Index(title, " - "); idx > 0 && idx < len(title)-3 {
			title = title[:idx]
		}
		return title
	}
	return ""
}

func inferImpetusFromURL(url string) string {
	switch {
	case strings.Contains(url, "github.com"):
		return "GitHub discovery"
	case strings.Contains(url, "twitter.com") || strings.Contains(url, "x.com"):
		return "X discovery"
	case strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be"):
		return "YouTube discovery"
	case strings.Contains(url, "linkedin.com"):
		return "LinkedIn discovery"
	case strings.Contains(url, "reddit.com"):
		return "Reddit discovery"
	case strings.Contains(url, "news.ycombinator.com"):
		return "HN discovery"
	default:
		return "Web discovery"
	}
}
