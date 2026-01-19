package entity

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/bierlingm/beats/internal/beat"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>\[\]"']+`)
var capitalizedNamePattern = regexp.MustCompile(`\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+)+)\b`)

// WALDConfig represents the WALD.yaml structure for entity extraction
type WALDConfig struct {
	Directories []WALDDirectory `yaml:"directories"`
}

type WALDDirectory struct {
	Path    string `yaml:"path"`
	Purpose string `yaml:"purpose"`
}

// ExtractEntities extracts entities from beat content using WALD.yaml data
func ExtractEntities(content string, werkRoot string) []beat.Entity {
	var entities []beat.Entity
	seen := make(map[string]bool)

	// Extract URLs (highest confidence)
	urls := urlPattern.FindAllString(content, -1)
	for _, url := range urls {
		key := "url:" + url
		if !seen[key] {
			seen[key] = true
			entities = append(entities, beat.Entity{
				Label:    url,
				Category: "url",
				Meta: map[string]string{
					"confidence": "1.0",
				},
			})
		}
	}

	// Load WALD config for cooperators and directories
	wald := loadWALDConfig(werkRoot)
	if wald != nil {
		// Extract cooperator mentions
		cooperators := extractCooperators(wald)
		for slug, displayName := range cooperators {
			if containsPersonName(content, displayName) || containsPersonName(content, slug) {
				key := "person:" + slug
				if !seen[key] {
					seen[key] = true
					entities = append(entities, beat.Entity{
						Label:    displayName,
						Category: "person",
						Meta: map[string]string{
							"confidence": "0.95",
							"cooperator": "cooperators/" + slug,
						},
					})
				}
			}
		}

		// Extract project/directory mentions
		for _, dir := range wald.Directories {
			dirName := filepath.Base(dir.Path)
			if containsProject(content, dirName, dir.Purpose) {
				key := "project:" + dir.Path
				if !seen[key] {
					seen[key] = true
					entities = append(entities, beat.Entity{
						Label:    dirName,
						Category: "project",
						Meta: map[string]string{
							"confidence": "0.90",
							"directory":  dir.Path,
						},
					})
				}
			}
		}
	}

	// Extract topics (significant capitalized phrases not matching known entities)
	topics := extractTopics(content, seen)
	for _, topic := range topics {
		key := "topic:" + strings.ToLower(topic)
		if !seen[key] {
			seen[key] = true
			entities = append(entities, beat.Entity{
				Label:    topic,
				Category: "topic",
				Meta: map[string]string{
					"confidence": "0.85",
				},
			})
		}
	}

	return entities
}

func loadWALDConfig(werkRoot string) *WALDConfig {
	if werkRoot == "" {
		werkRoot = findWerkRoot()
	}
	if werkRoot == "" {
		return nil
	}

	waldPath := filepath.Join(werkRoot, "WALD.yaml")
	data, err := os.ReadFile(waldPath)
	if err != nil {
		return nil
	}

	var config WALDConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil
	}

	return &config
}

func findWerkRoot() string {
	if root := os.Getenv("BEATS_ROOT"); root != "" {
		if _, err := os.Stat(filepath.Join(root, "WALD.yaml")); err == nil {
			return root
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "WALD.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	home, _ := os.UserHomeDir()
	werkPath := filepath.Join(home, "werk")
	if _, err := os.Stat(filepath.Join(werkPath, "WALD.yaml")); err == nil {
		return werkPath
	}

	return ""
}

func extractCooperators(wald *WALDConfig) map[string]string {
	cooperators := make(map[string]string)
	for _, dir := range wald.Directories {
		if strings.HasPrefix(dir.Path, "cooperators/") {
			slug := strings.TrimPrefix(dir.Path, "cooperators/")
			displayName := slugToDisplayName(slug)
			cooperators[slug] = displayName
		}
	}
	return cooperators
}

func slugToDisplayName(slug string) string {
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func containsPersonName(content, name string) bool {
	contentLower := strings.ToLower(content)
	nameLower := strings.ToLower(name)

	if strings.Contains(contentLower, nameLower) {
		return true
	}

	// Check for first name only (for known cooperators)
	parts := strings.Fields(name)
	if len(parts) > 0 {
		firstName := strings.ToLower(parts[0])
		// Only match first name if it's at least 4 chars to avoid false positives
		if len(firstName) >= 4 && strings.Contains(contentLower, firstName) {
			return true
		}
	}

	return false
}

func containsProject(content, dirName, purpose string) bool {
	contentLower := strings.ToLower(content)

	// Match directory name
	if len(dirName) >= 3 && strings.Contains(contentLower, strings.ToLower(dirName)) {
		return true
	}

	// Extract key terms from purpose for matching
	purposeTerms := extractPurposeTerms(purpose)
	for _, term := range purposeTerms {
		if len(term) >= 4 && strings.Contains(contentLower, strings.ToLower(term)) {
			return true
		}
	}

	return false
}

func extractPurposeTerms(purpose string) []string {
	// Extract significant terms from purpose description
	words := strings.Fields(purpose)
	var terms []string
	for _, word := range words {
		word = strings.Trim(word, "().,;:\"'")
		// Keep proper nouns and significant terms
		if len(word) >= 4 && (word[0] >= 'A' && word[0] <= 'Z') {
			terms = append(terms, word)
		}
	}
	return terms
}

func extractTopics(content string, seen map[string]bool) []string {
	var topics []string

	// Find capitalized phrases that might be topics
	matches := capitalizedNamePattern.FindAllString(content, -1)
	for _, match := range matches {
		matchLower := strings.ToLower(match)
		// Skip if already captured as person/project
		if seen["person:"+matchLower] || seen["project:"+matchLower] {
			continue
		}
		// Skip common words
		if isCommonPhrase(match) {
			continue
		}
		topics = append(topics, match)
	}

	// Limit to top 3 topics
	if len(topics) > 3 {
		topics = topics[:3]
	}

	return topics
}

var commonPhrases = map[string]bool{
	"The": true, "This": true, "That": true, "These": true, "Those": true,
	"What": true, "When": true, "Where": true, "Which": true, "Who": true,
	"How": true, "Why": true, "Some": true, "Many": true, "Most": true,
}

func isCommonPhrase(phrase string) bool {
	words := strings.Fields(phrase)
	if len(words) == 0 {
		return true
	}
	return commonPhrases[words[0]]
}
