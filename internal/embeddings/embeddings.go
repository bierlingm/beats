package embeddings

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bierlingm/beats/internal/beat"
)

const (
	EmbeddingDimensions = 768 // nomic-embed-text
	embeddingsFile      = "embeddings.bin"
	indexFile           = "embeddings.idx"
	DefaultOllamaURL    = "http://localhost:11434"
	EmbeddingModel      = "nomic-embed-text"
)

// Store manages embedding storage
type Store struct {
	dir   string
	index map[string]int64
}

// NewStore creates or loads an embedding store
func NewStore(beatsDir string) (*Store, error) {
	s := &Store{
		dir:   beatsDir,
		index: make(map[string]int64),
	}
	if err := s.loadIndex(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *Store) binPath() string { return filepath.Join(s.dir, embeddingsFile) }
func (s *Store) idxPath() string { return filepath.Join(s.dir, indexFile) }

func (s *Store) loadIndex() error {
	data, err := os.ReadFile(s.idxPath())
	if err != nil {
		return err
	}
	s.index = make(map[string]int64)
	pos := 0
	for pos < len(data) {
		if pos+4 > len(data) {
			break
		}
		idLen := int(binary.LittleEndian.Uint32(data[pos:]))
		pos += 4
		if pos+idLen+8 > len(data) {
			break
		}
		id := string(data[pos : pos+idLen])
		pos += idLen
		offset := int64(binary.LittleEndian.Uint64(data[pos:]))
		pos += 8
		s.index[id] = offset
	}
	return nil
}

func (s *Store) saveIndex() error {
	var buf []byte
	for id, offset := range s.index {
		idBytes := []byte(id)
		lenBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBuf, uint32(len(idBytes)))
		buf = append(buf, lenBuf...)
		buf = append(buf, idBytes...)
		offsetBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBuf, uint64(offset))
		buf = append(buf, offsetBuf...)
	}
	return os.WriteFile(s.idxPath(), buf, 0644)
}

func (s *Store) Has(beatID string) bool {
	_, ok := s.index[beatID]
	return ok
}

func (s *Store) Store(beatID string, embedding []float64) error {
	if len(embedding) != EmbeddingDimensions {
		return fmt.Errorf("expected %d dimensions, got %d", EmbeddingDimensions, len(embedding))
	}
	f, err := os.OpenFile(s.binPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()

	buf := make([]byte, EmbeddingDimensions*8)
	for i, v := range embedding {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v))
	}
	if _, err := f.Write(buf); err != nil {
		return err
	}
	s.index[beatID] = offset
	return s.saveIndex()
}

func (s *Store) Get(beatID string) ([]float64, error) {
	offset, ok := s.index[beatID]
	if !ok {
		return nil, fmt.Errorf("no embedding for %s", beatID)
	}
	f, err := os.Open(s.binPath())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, EmbeddingDimensions*8)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	embedding := make([]float64, EmbeddingDimensions)
	for i := range embedding {
		bits := binary.LittleEndian.Uint64(buf[i*8:])
		embedding[i] = math.Float64frombits(bits)
	}
	return embedding, nil
}

func (s *Store) Count() int { return len(s.index) }
func (s *Store) Coverage(total int) float64 {
	if total == 0 {
		return 100.0
	}
	return float64(len(s.index)) / float64(total) * 100.0
}

// OllamaClient for embeddings
type OllamaClient struct {
	baseURL string
	client  *http.Client
}

func NewOllamaClient() *OllamaClient {
	return &OllamaClient{
		baseURL: DefaultOllamaURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *OllamaClient) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func (c *OllamaClient) GetEmbedding(ctx context.Context, text string) ([]float64, error) {
	reqBody, _ := json.Marshal(map[string]string{"model": EmbeddingModel, "prompt": text})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/embeddings", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

// ComputeResult for batch computation
type ComputeResult struct {
	Computed int
	Skipped  int
	Errors   int
}

func ComputeMissing(ctx context.Context, beats []beat.Beat, store *Store, ollama *OllamaClient) (*ComputeResult, error) {
	result := &ComputeResult{}
	if !ollama.IsAvailable() {
		return nil, fmt.Errorf("ollama not available")
	}
	for _, b := range beats {
		if store.Has(b.ID) {
			result.Skipped++
			continue
		}
		text := b.Content
		if b.Impetus.Label != "" {
			text = b.Impetus.Label + ": " + text
		}
		embedding, err := ollama.GetEmbedding(ctx, text)
		if err != nil {
			result.Errors++
			continue
		}
		if err := store.Store(b.ID, embedding); err != nil {
			result.Errors++
			continue
		}
		result.Computed++
	}
	return result, nil
}

// SearchResult for semantic search
type SearchResult struct {
	ID      string
	Score   float64
	Content string
	Impetus beat.Impetus
}

func SemanticSearch(ctx context.Context, query string, beats []beat.Beat, store *Store, ollama *OllamaClient, limit int) ([]SearchResult, error) {
	queryEmb, err := ollama.GetEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, b := range beats {
		beatEmb, err := store.Get(b.ID)
		if err != nil {
			continue
		}
		sim := cosineSimilarity(queryEmb, beatEmb)
		results = append(results, SearchResult{
			ID:      b.ID,
			Score:   sim,
			Content: b.Content,
			Impetus: b.Impetus,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
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
