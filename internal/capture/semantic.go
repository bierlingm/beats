package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const purposeCacheFile = ".wald/purpose-embeddings.json"

type SemanticInference struct {
	werkRoot string
	cache    *PurposeEmbeddingsCache
	ollama   *OllamaClient
}

type PurposeEmbeddingsCache struct {
	Directories map[string][]float64 `json:"directories"`
	UpdatedAt   string               `json:"updated_at"`
}

type InferredContext struct {
	WALDDirectory   string
	InferenceMethod string
	Confidence      float64
}

type OllamaClient struct {
	baseURL string
}

type waldDirectory struct {
	Path    string
	Purpose string
	State   string
}

func NewSemanticInference(werkRoot string) *SemanticInference {
	return &SemanticInference{
		werkRoot: werkRoot,
		ollama:   &OllamaClient{baseURL: "http://localhost:11434"},
	}
}

func (s *SemanticInference) InferContext(beatContent string) (*InferredContext, error) {
	if !s.isOllamaAvailable() {
		return nil, nil
	}

	beatEmb, err := s.getEmbedding(beatContent)
	if err != nil {
		return nil, nil
	}

	if err := s.ensureCache(); err != nil {
		return nil, nil
	}

	var bestDir string
	var bestScore float64

	for dir, dirEmb := range s.cache.Directories {
		score := cosineSimilarity(beatEmb, dirEmb)
		if score > bestScore {
			bestScore = score
			bestDir = dir
		}
	}

	if bestDir == "" || bestScore < 0.3 {
		return nil, nil
	}

	return &InferredContext{
		WALDDirectory:   bestDir,
		InferenceMethod: "semantic",
		Confidence:      bestScore,
	}, nil
}

func (s *SemanticInference) isOllamaAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := newRequestWithContext(ctx, "GET", s.ollama.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == 200
}

func (s *SemanticInference) getEmbedding(text string) ([]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"model": "nomic-embed-text", "prompt": text})
	req, err := newRequestWithContext(ctx, "POST", s.ollama.baseURL+"/api/embeddings", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func (s *SemanticInference) ensureCache() error {
	cachePath := filepath.Join(s.werkRoot, purposeCacheFile)

	if data, err := os.ReadFile(cachePath); err == nil {
		var cache PurposeEmbeddingsCache
		if json.Unmarshal(data, &cache) == nil && len(cache.Directories) > 0 {
			s.cache = &cache
			return nil
		}
	}

	return s.rebuildCache()
}

func (s *SemanticInference) rebuildCache() error {
	waldPath := filepath.Join(s.werkRoot, "WALD.yaml")
	dirs, err := parseWALDDirectories(waldPath)
	if err != nil {
		return err
	}

	s.cache = &PurposeEmbeddingsCache{
		Directories: make(map[string][]float64),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	for _, dir := range dirs {
		if dir.Purpose == "" || dir.State == "archived" {
			continue
		}
		emb, err := s.getEmbedding(dir.Purpose)
		if err != nil {
			continue
		}
		s.cache.Directories[dir.Path] = emb
	}

	cacheDir := filepath.Dir(filepath.Join(s.werkRoot, purposeCacheFile))
	_ = os.MkdirAll(cacheDir, 0755)

	cacheData, _ := json.MarshalIndent(s.cache, "", "  ")
	return os.WriteFile(filepath.Join(s.werkRoot, purposeCacheFile), cacheData, 0644)
}

// parseWALDDirectories parses WALD.yaml for directories with path, purpose, state.
func parseWALDDirectories(waldPath string) ([]waldDirectory, error) {
	f, err := os.Open(waldPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var dirs []waldDirectory
	var current *waldDirectory
	inDirectories := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "directories:") {
			inDirectories = true
			continue
		}

		if !inDirectories {
			continue
		}

		// New directory entry starts with "- path:"
		if strings.HasPrefix(trimmed, "- path:") {
			if current != nil {
				dirs = append(dirs, *current)
			}
			current = &waldDirectory{
				Path: strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")),
			}
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(trimmed, "purpose:") {
			purpose := strings.TrimPrefix(trimmed, "purpose:")
			purpose = strings.TrimSpace(purpose)
			purpose = strings.Trim(purpose, "\"")
			current.Purpose = purpose
		} else if strings.HasPrefix(trimmed, "state:") {
			current.State = strings.TrimSpace(strings.TrimPrefix(trimmed, "state:"))
		}
	}

	if current != nil {
		dirs = append(dirs, *current)
	}

	return dirs, scanner.Err()
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
