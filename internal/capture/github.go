package capture

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// GitHubCapture represents captured content from GitHub
type GitHubCapture struct {
	Owner       string
	Repo        string
	Description string
	Stars       int
	URL         string
	Content     string
}

// CaptureFromGitHub fetches repo info from GitHub API
func CaptureFromGitHub(ref string, additionalContent string) (*GitHubCapture, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GitHub reference, use owner/repo format")
	}

	owner, repo := parts[0], parts[1]
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		// Fallback without API data
		return &GitHubCapture{
			Owner:   owner,
			Repo:    repo,
			URL:     fmt.Sprintf("https://github.com/%s/%s", owner, repo),
			Content: fmt.Sprintf("%s/%s\n\nhttps://github.com/%s/%s", owner, repo, owner, repo),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &GitHubCapture{
			Owner:   owner,
			Repo:    repo,
			URL:     fmt.Sprintf("https://github.com/%s/%s", owner, repo),
			Content: fmt.Sprintf("%s/%s\n\nhttps://github.com/%s/%s", owner, repo, owner, repo),
		}, nil
	}

	var data struct {
		Description string `json:"description"`
		Stars       int    `json:"stargazers_count"`
		HTMLURL     string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		data.HTMLURL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	}

	capture := &GitHubCapture{
		Owner:       owner,
		Repo:        repo,
		Description: data.Description,
		Stars:       data.Stars,
		URL:         data.HTMLURL,
	}

	if additionalContent != "" {
		capture.Content = fmt.Sprintf("%s\n\n%s/%s - %s (⭐ %d)\n%s",
			additionalContent, owner, repo, data.Description, data.Stars, data.HTMLURL)
	} else {
		capture.Content = fmt.Sprintf("%s/%s - %s\n\n%s (⭐ %d)",
			owner, repo, data.Description, data.HTMLURL, data.Stars)
	}

	return capture, nil
}
