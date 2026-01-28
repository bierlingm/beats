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
	BeatsDirEnvVar   = "BEATS_DIR"
	// GlobalBeatsStore is the canonical single store for all beats in werk.
	// This replaces the scattered per-directory .beats/ stores.
	GlobalBeatsStore = "/Users/moritzbierling/werk/.beats"
)

// JSONLStore manages beats in an append-only JSONL file.
type JSONLStore struct {
	dir      string
	filePath string
	mu       sync.RWMutex
}

// isValidBeatsDir checks if a directory is a valid .beats directory.
// A valid .beats directory must exist and either:
// - contain a beats.jsonl file, OR
// - contain a hooks.json file (initialized but empty)
func isValidBeatsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	// Check for beats.jsonl (has data)
	if _, err := os.Stat(filepath.Join(path, DefaultBeatsFile)); err == nil {
		return true
	}
	// Check for hooks.json (initialized project)
	if _, err := os.Stat(filepath.Join(path, "hooks.json")); err == nil {
		return true
	}
	return false
}

// findBeatsDir walks up from startDir to find an existing .beats directory.
// Only considers directories with actual beats data (beats.jsonl or hooks.json).
// Returns the first valid .beats found, or startDir/.beats if none exists.
func findBeatsDir(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, DefaultBeatsDir)
		if isValidBeatsDir(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root, use startDir
			return filepath.Join(startDir, DefaultBeatsDir)
		}
		dir = parent
	}
}

// GetBeatsDir returns the beats directory path with the following precedence:
// 1. BEATS_DIR environment variable (if set)
// 2. Global beats store at ~/werk/.beats/ (canonical single store)
func GetBeatsDir() (string, error) {
	// Check BEATS_DIR environment variable first
	if envDir := os.Getenv(BeatsDirEnvVar); envDir != "" {
		return envDir, nil
	}

	// Use the global beats store - all beats go to one place
	return GlobalBeatsStore, nil
}

// DiscoverBeatsProjects finds all valid .beats directories under the given root.
// Skips hidden directories (except .beats itself) and common non-project dirs.
func DiscoverBeatsProjects(root string) ([]string, error) {
	var projects []string
	skipDirs := map[string]bool{
		"node_modules": true,
		".git":         true,
		"vendor":       true,
		"__pycache__":  true,
		".cache":       true,
		".npm":         true,
		".cargo":       true,
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		// Skip hidden directories (except when we find .beats)
		if strings.HasPrefix(name, ".") && name != DefaultBeatsDir {
			return filepath.SkipDir
		}

		// Skip common non-project directories
		if skipDirs[name] {
			return filepath.SkipDir
		}

		// Check if this is a valid .beats directory
		if name == DefaultBeatsDir && isValidBeatsDir(path) {
			projects = append(projects, path)
			return filepath.SkipDir // Don't descend into .beats
		}

		return nil
	})

	return projects, err
}

// ProjectInfo contains metadata about a beats project.
type ProjectInfo struct {
	BeatsDir    string
	ProjectName string
	BeatCount   int
}

// GetProjectInfo returns info about a .beats directory.
func GetProjectInfo(beatsDir string) (*ProjectInfo, error) {
	store, err := NewJSONLStore(beatsDir)
	if err != nil {
		return nil, err
	}

	beats, err := store.ReadAll()
	if err != nil {
		return nil, err
	}

	// Extract project name from path (parent directory name)
	projectName := filepath.Base(filepath.Dir(beatsDir))

	return &ProjectInfo{
		BeatsDir:    beatsDir,
		ProjectName: projectName,
		BeatCount:   len(beats),
	}, nil
}

// NewJSONLStore creates a new JSONL store.
// If dir is empty, uses GetBeatsDir() to find or create the beats directory.
// This walks up from cwd to find an existing .beats folder (like git finds .git).
func NewJSONLStore(dir string) (*JSONLStore, error) {
	if dir == "" {
		var err error
		dir, err = GetBeatsDir()
		if err != nil {
			return nil, err
		}
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
	return s.NextSequenceForDate(time.Now().UTC())
}

// NextSequenceForDate returns the next sequence number for beats on a specific date.
func (s *JSONLStore) NextSequenceForDate(date time.Time) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	beats, err := s.readAllUnlocked()
	if err != nil {
		return 1, err
	}

	dateStr := date.UTC().Format("20060102")
	prefix := fmt.Sprintf("beat-%s-", dateStr)

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

// MostRecent returns the most recently created beat.
func (s *JSONLStore) MostRecent() (*beat.Beat, error) {
	beats, err := s.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(beats) == 0 {
		return nil, fmt.Errorf("no beats found")
	}

	mostRecent := &beats[0]
	for i := range beats {
		if beats[i].CreatedAt.After(mostRecent.CreatedAt) {
			mostRecent = &beats[i]
		}
	}

	return mostRecent, nil
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

// Delete removes a beat by ID.
func (s *JSONLStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	beats, err := s.readAllUnlocked()
	if err != nil {
		return err
	}

	found := false
	filtered := make([]beat.Beat, 0, len(beats)-1)
	for _, b := range beats {
		if b.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, b)
	}

	if !found {
		return fmt.Errorf("beat not found: %s", id)
	}

	return s.rewriteUnlocked(filtered)
}

// BeatExists checks if a beat with the given ID already exists.
func (s *JSONLStore) BeatExists(id string) (bool, error) {
	beats, err := s.ReadAll()
	if err != nil {
		return false, err
	}
	for _, b := range beats {
		if b.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// AppendBulk appends multiple beats to the store in a single operation.
func (s *JSONLStore) AppendBulk(beats []*beat.Beat) error {
	if len(beats) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open beats file: %w", err)
	}
	defer f.Close()

	for _, b := range beats {
		data, err := json.Marshal(b)
		if err != nil {
			return fmt.Errorf("failed to marshal beat %s: %w", b.ID, err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write beat %s: %w", b.ID, err)
		}
	}

	return nil
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
