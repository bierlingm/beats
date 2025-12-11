package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bierlingm/beats/internal/beat"
	"github.com/bierlingm/beats/internal/hooks"
)

const (
	DefaultBeatsDir  = ".beats"
	DefaultBeatsFile = "beats.jsonl"
)

// JSONLStore manages beats in an append-only JSONL file.
type JSONLStore struct {
	dir      string
	filePath string
	mu       sync.RWMutex
}

// NewJSONLStore creates a new JSONL store.
// If dir is empty, uses the current directory's .beats folder.
func NewJSONLStore(dir string) (*JSONLStore, error) {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dir = filepath.Join(cwd, DefaultBeatsDir)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create beats directory: %w", err)
	}

	return &JSONLStore{
		dir:      dir,
		filePath: filepath.Join(dir, DefaultBeatsFile),
	}, nil
}

// Append adds a new beat to the store.
func (s *JSONLStore) Append(b *beat.Beat) error {
	s.mu.Lock()

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to open beats file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(b)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to marshal beat: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to write beat: %w", err)
	}

	// Read all beats while still holding the lock
	allBeats, _ := s.readAllUnlocked()
	s.mu.Unlock()

	// Trigger hooks synchronously (fast enough, goroutine was exiting before completion)
	s.triggerHooks(b, allBeats)

	return nil
}

// triggerHooks runs hook checks after a beat is added.
func (s *JSONLStore) triggerHooks(newBeat *beat.Beat, allBeats []beat.Beat) {
	hookMgr, err := hooks.NewManager(s.dir)
	if err != nil {
		return // Silently ignore hook errors
	}

	// Fire-and-forget: hook errors don't affect beat storage
	_ = hookMgr.OnBeatAdded(newBeat, allBeats)
}

// ReadAll reads all beats from the store.
func (s *JSONLStore) ReadAll() ([]beat.Beat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.readAllUnlocked()
}

func (s *JSONLStore) readAllUnlocked() ([]beat.Beat, error) {
	f, err := os.Open(s.filePath)
	if os.IsNotExist(err) {
		return []beat.Beat{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open beats file: %w", err)
	}
	defer f.Close()

	var beats []beat.Beat
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var b beat.Beat
		if err := json.Unmarshal([]byte(line), &b); err != nil {
			return nil, fmt.Errorf("failed to parse beat at line %d: %w", lineNum, err)
		}
		beats = append(beats, b)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read beats file: %w", err)
	}

	return beats, nil
}

// Get retrieves a beat by ID.
func (s *JSONLStore) Get(id string) (*beat.Beat, error) {
	beats, err := s.ReadAll()
	if err != nil {
		return nil, err
	}

	for i := range beats {
		if beats[i].ID == id {
			return &beats[i], nil
		}
	}

	return nil, fmt.Errorf("beat not found: %s", id)
}

// NextSequence returns the next sequence number for today's beats.
func (s *JSONLStore) NextSequence() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	beats, err := s.readAllUnlocked()
	if err != nil {
		return 1, err
	}

	today := time.Now().UTC().Format("20060102")
	prefix := fmt.Sprintf("beat-%s-", today)

	maxSeq := 0
	for _, b := range beats {
		if strings.HasPrefix(b.ID, prefix) {
			seqStr := strings.TrimPrefix(b.ID, prefix)
			if seq, err := strconv.Atoi(seqStr); err == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	}

	return maxSeq + 1, nil
}

// Search performs a simple keyword search across beat content and impetus.
func (s *JSONLStore) Search(query string, maxResults int) ([]beat.SearchResult, error) {
	beats, err := s.ReadAll()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []beat.SearchResult

	for _, b := range beats {
		contentLower := strings.ToLower(b.Content)
		labelLower := strings.ToLower(b.Impetus.Label)

		score := 0.0
		if strings.Contains(contentLower, query) {
			score += 0.5
		}
		if strings.Contains(labelLower, query) {
			score += 0.5
		}

		if score > 0 {
			results = append(results, beat.SearchResult{
				ID:      b.ID,
				Score:   score,
				Content: b.Content,
				Impetus: b.Impetus,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}

	return results, nil
}

// GetSince returns all beats created or modified since the given time.
func (s *JSONLStore) GetSince(since time.Time) (new, modified, linked []beat.Beat, err error) {
	beats, err := s.ReadAll()
	if err != nil {
		return nil, nil, nil, err
	}

	for _, b := range beats {
		if b.CreatedAt.After(since) || b.CreatedAt.Equal(since) {
			new = append(new, b)
		} else if b.UpdatedAt.After(since) || b.UpdatedAt.Equal(since) {
			modified = append(modified, b)
		}
		if len(b.LinkedBeads) > 0 && (b.UpdatedAt.After(since) || b.UpdatedAt.Equal(since)) {
			linked = append(linked, b)
		}
	}

	return new, modified, linked, nil
}

// GetByIDs returns beats matching the given IDs.
func (s *JSONLStore) GetByIDs(ids []string) ([]beat.Beat, error) {
	beats, err := s.ReadAll()
	if err != nil {
		return nil, err
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	var result []beat.Beat
	for _, b := range beats {
		if idSet[b.ID] {
			result = append(result, b)
		}
	}

	return result, nil
}

// GetByLinkedBead returns beats linked to a specific bead ID.
func (s *JSONLStore) GetByLinkedBead(beadID string) ([]beat.Beat, error) {
	beats, err := s.ReadAll()
	if err != nil {
		return nil, err
	}

	var result []beat.Beat
	for _, b := range beats {
		for _, linked := range b.LinkedBeads {
			if linked == beadID {
				result = append(result, b)
				break
			}
		}
	}

	return result, nil
}

// Path returns the path to the JSONL file.
func (s *JSONLStore) Path() string {
	return s.filePath
}

// Dir returns the beats directory path.
func (s *JSONLStore) Dir() string {
	return s.dir
}

// Update modifies a beat in place by rewriting the JSONL file.
// The updater function receives a pointer to the beat and can modify it.
func (s *JSONLStore) Update(id string, updater func(*beat.Beat) error) (*beat.Beat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	beats, err := s.readAllUnlocked()
	if err != nil {
		return nil, err
	}

	var updated *beat.Beat
	found := false
	for i := range beats {
		if beats[i].ID == id {
			if err := updater(&beats[i]); err != nil {
				return nil, fmt.Errorf("updater failed: %w", err)
			}
			beats[i].UpdatedAt = time.Now().UTC()
			updated = &beats[i]
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("beat not found: %s", id)
	}

	// Rewrite the entire file
	if err := s.rewriteUnlocked(beats); err != nil {
		return nil, err
	}

	return updated, nil
}

// rewriteUnlocked rewrites the JSONL file with the given beats.
// Caller must hold the write lock.
func (s *JSONLStore) rewriteUnlocked(beats []beat.Beat) error {
	// Write to temp file first for atomicity
	tmpPath := s.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	for _, b := range beats {
		data, err := json.Marshal(b)
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to marshal beat %s: %w", b.ID, err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to write beat %s: %w", b.ID, err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
