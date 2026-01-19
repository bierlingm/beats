package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
)

const (
	defaultOllamaURL    = "http://localhost:11434"
	defaultEmbedModel   = "embeddinggemma"
	embeddingsCacheFile = "embeddings_cache.json"
)

// SemanticSearcher provides semantic search via Ollama embeddings.
type SemanticSearcher struct {
	jsonl     *JSONLStore
	cacheDir  string
	ollamaURL string
	model     string
	cache     map[string][]float64
}

// NewSemanticSearcher creates a new semantic searcher using Ollama.
func NewSemanticSearcher(jsonl *JSONLStore) (*SemanticSearcher, error) {
	cacheDir := filepath.Join(jsonl.Dir(), ".semantic_cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	s := &SemanticSearcher{
		jsonl:     jsonl,
		cacheDir:  cacheDir,
		ollamaURL: defaultOllamaURL,
		model:     defaultEmbedModel,
		cache:     make(map[string][]float64),
	}

	s.loadCache()
	return s, nil
}

// Available checks if Ollama is running and has an embedding model.
func (s *SemanticSearcher) Available() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(s.ollamaURL + "/api/tags")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == 200
}

func (s *SemanticSearcher) loadCache() {
	data, err := os.ReadFile(filepath.Join(s.cacheDir, embeddingsCacheFile))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.cache)
}

func (s *SemanticSearcher) saveCache() {
	data, _ := json.Marshal(s.cache)
	_ = os.WriteFile(filepath.Join(s.cacheDir, embeddingsCacheFile), data, 0644)
}

// getEmbedding fetches embedding from Ollama or cache.
func (s *SemanticSearcher) getEmbedding(text string) ([]float64, error) {
	cacheKey := fmt.Sprintf("%x", text)[:32]
	if emb, ok := s.cache[cacheKey]; ok {
		return emb, nil
	}

	reqBody := map[string]interface{}{
		"model":  s.model,
		"prompt": text,
	}
	jsonBody, _ := json.Marshal(reqBody)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(s.ollamaURL+"/api/embeddings", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	s.cache[cacheKey] = result.Embedding
	return result.Embedding, nil
}

// cosineSimilarity calculates similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// formatBeatText creates searchable text from a beat.
func formatBeatText(b beat.Beat) string {
	parts := []string{b.Impetus.Label, b.Content}
	for _, e := range b.Entities {
		parts = append(parts, e.Label)
	}
	return strings.Join(parts, " ")
}

// Search performs semantic search using Ollama embeddings.
func (s *SemanticSearcher) Search(query string, maxResults int) ([]beat.SearchResult, error) {
	queryEmb, err := s.getEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	beats, err := s.jsonl.ReadAll()
	if err != nil {
		return nil, err
	}

	type scoredBeat struct {
		beat  beat.Beat
		score float64
	}
	var scored []scoredBeat

	for _, b := range beats {
		text := formatBeatText(b)
		beatEmb, err := s.getEmbedding(text)
		if err != nil {
			continue
		}

		score := cosineSimilarity(queryEmb, beatEmb)
		scored = append(scored, scoredBeat{beat: b, score: score})
	}

	s.saveCache()

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	var results []beat.SearchResult
	for _, sb := range scored {
		results = append(results, beat.SearchResult{
			ID:      sb.beat.ID,
			Score:   sb.score,
			Content: sb.beat.Content,
			Impetus: sb.beat.Impetus,
		})
	}

	return results, nil
}

// SemanticSearchInput extends SearchInput with semantic options.
type SemanticSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
	Semantic   bool   `json:"semantic,omitempty"`
}

// SemanticSearchOutput extends SearchOutput with search mode info.
type SemanticSearchOutput struct {
	Results  []beat.SearchResult `json:"results"`
	Mode     string              `json:"mode"`
	Fallback bool                `json:"fallback,omitempty"`
}

// HybridSearch performs semantic search with FTS5 fallback.
func HybridSearch(jsonl *JSONLStore, query string, maxResults int, semantic bool) (*SemanticSearchOutput, error) {
	if !semantic {
		results, err := jsonl.Search(query, maxResults)
		if err != nil {
			return nil, err
		}
		return &SemanticSearchOutput{
			Results: results,
			Mode:    "keyword",
		}, nil
	}

	searcher, err := NewSemanticSearcher(jsonl)
	if err != nil {
		results, err := jsonl.Search(query, maxResults)
		if err != nil {
			return nil, err
		}
		return &SemanticSearchOutput{
			Results:  results,
			Mode:     "keyword",
			Fallback: true,
		}, nil
	}

	if !searcher.Available() {
		results, err := jsonl.Search(query, maxResults)
		if err != nil {
			return nil, err
		}
		return &SemanticSearchOutput{
			Results:  results,
			Mode:     "keyword",
			Fallback: true,
		}, nil
	}

	results, err := searcher.Search(query, maxResults)
	if err != nil {
		results, err := jsonl.Search(query, maxResults)
		if err != nil {
			return nil, err
		}
		return &SemanticSearchOutput{
			Results:  results,
			Mode:     "keyword",
			Fallback: true,
		}, nil
	}

	return &SemanticSearchOutput{
		Results: results,
		Mode:    "semantic",
	}, nil
}

// Status returns semantic search availability info.
func SemanticStatus() map[string]interface{} {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(defaultOllamaURL + "/api/tags")
	available := err == nil && resp != nil && resp.StatusCode == 200
	if resp != nil {
		_ = resp.Body.Close()
	}

	return map[string]interface{}{
		"available":    available,
		"backend":      "ollama",
		"model":        defaultEmbedModel,
		"capabilities": []string{"semantic_search", "embedding_similarity"},
	}
}

func SemanticStatusJSON() ([]byte, error) {
	return json.Marshal(SemanticStatus())
}
