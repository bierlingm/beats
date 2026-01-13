package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bierlingm/beats/internal/beat"
)

func TestJSONLStore_AppendAndReadAll(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	b := beat.NewBeat("test content", beat.Impetus{Label: "test"})

	if err := store.Append(b); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	beats, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(beats) != 1 {
		t.Errorf("ReadAll() returned %d beats, want 1", len(beats))
	}

	if beats[0].Content != "test content" {
		t.Errorf("Content = %q, want %q", beats[0].Content, "test content")
	}
}

func TestJSONLStore_Get(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	b := beat.NewBeat("test content", beat.Impetus{Label: "test"})
	if err := store.Append(b); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	got, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID != b.ID {
		t.Errorf("Get() ID = %q, want %q", got.ID, b.ID)
	}
}

func TestJSONLStore_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	_, err = store.Get("nonexistent")
	if err == nil {
		t.Error("Get() expected error for nonexistent beat")
	}
}

func TestJSONLStore_Search(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	b1 := beat.NewBeat("coaching session notes", beat.Impetus{Label: "coaching"})
	b2 := beat.NewBeat("random thoughts", beat.Impetus{Label: "journal"})

	store.Append(b1)
	store.Append(b2)

	results, err := store.Search("coaching", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Search() returned %d results, want 1", len(results))
	}
}

func TestJSONLStore_Update(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	b := beat.NewBeat("original content", beat.Impetus{Label: "test"})
	if err := store.Append(b); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	updated, err := store.Update(b.ID, func(b *beat.Beat) error {
		b.LinkedBeads = append(b.LinkedBeads, "bead-123")
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(updated.LinkedBeads) != 1 || updated.LinkedBeads[0] != "bead-123" {
		t.Errorf("Update() LinkedBeads = %v, want [bead-123]", updated.LinkedBeads)
	}

	// Verify persistence
	got, _ := store.Get(b.ID)
	if len(got.LinkedBeads) != 1 {
		t.Errorf("After reload, LinkedBeads = %v, want [bead-123]", got.LinkedBeads)
	}
}

func TestJSONLStore_NextSequence(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	seq1, _ := store.NextSequence()
	if seq1 != 1 {
		t.Errorf("NextSequence() = %d, want 1", seq1)
	}

	b := beat.NewBeat("test", beat.Impetus{Label: "test"})
	store.Append(b)

	seq2, _ := store.NextSequence()
	if seq2 != 2 {
		t.Errorf("NextSequence() after append = %d, want 2", seq2)
	}
}

func TestJSONLStore_Dir(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	if store.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", store.Dir(), dir)
	}
}

func TestJSONLStore_Path(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	expected := filepath.Join(dir, DefaultBeatsFile)
	if store.Path() != expected {
		t.Errorf("Path() = %q, want %q", store.Path(), expected)
	}
}

func TestJSONLStore_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	beatsDir := filepath.Join(dir, "subdir", ".beats")

	_, err := NewJSONLStore(beatsDir)
	if err != nil {
		t.Fatalf("NewJSONLStore() error = %v", err)
	}

	if _, err := os.Stat(beatsDir); os.IsNotExist(err) {
		t.Error("NewJSONLStore() did not create directory")
	}
}

func TestFindBeatsDir_WalksUpTree(t *testing.T) {
	// Create a temp directory structure:
	// root/
	//   .beats/           <- should find this (with beats.jsonl)
	//   subdir/
	//     nested/         <- start from here
	root := t.TempDir()
	beatsDir := filepath.Join(root, ".beats")
	nestedDir := filepath.Join(root, "subdir", "nested")

	if err := os.MkdirAll(beatsDir, 0755); err != nil {
		t.Fatalf("Failed to create .beats dir: %v", err)
	}
	// Create a beats.jsonl to make it a valid .beats directory
	if err := os.WriteFile(filepath.Join(beatsDir, "beats.jsonl"), []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create beats.jsonl: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	found := findBeatsDir(nestedDir)
	if found != beatsDir {
		t.Errorf("findBeatsDir() = %q, want %q", found, beatsDir)
	}
}

func TestFindBeatsDir_UsesClosest(t *testing.T) {
	// Create a temp directory structure:
	// root/
	//   .beats/           <- should NOT find this (valid but further)
	//   subdir/
	//     .beats/         <- should find this (closer, valid)
	//     nested/         <- start from here
	root := t.TempDir()
	rootBeats := filepath.Join(root, ".beats")
	subdirBeats := filepath.Join(root, "subdir", ".beats")
	nestedDir := filepath.Join(root, "subdir", "nested")

	if err := os.MkdirAll(rootBeats, 0755); err != nil {
		t.Fatalf("Failed to create root .beats dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootBeats, "beats.jsonl"), []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create root beats.jsonl: %v", err)
	}
	if err := os.MkdirAll(subdirBeats, 0755); err != nil {
		t.Fatalf("Failed to create subdir .beats dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdirBeats, "beats.jsonl"), []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create subdir beats.jsonl: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	found := findBeatsDir(nestedDir)
	if found != subdirBeats {
		t.Errorf("findBeatsDir() = %q, want %q (closest)", found, subdirBeats)
	}
}

func TestFindBeatsDir_FallsBackToStart(t *testing.T) {
	// Create a temp directory with no .beats anywhere
	root := t.TempDir()
	nestedDir := filepath.Join(root, "subdir", "nested")

	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	found := findBeatsDir(nestedDir)
	expected := filepath.Join(nestedDir, ".beats")
	if found != expected {
		t.Errorf("findBeatsDir() = %q, want %q (fallback)", found, expected)
	}
}

func TestFindBeatsDir_SkipsEmptyBeatsDir(t *testing.T) {
	// Create a temp directory structure:
	// root/
	//   .beats/beats.jsonl   <- should find this (valid)
	//   subdir/
	//     .beats/            <- should SKIP this (empty, no beats.jsonl)
	//     nested/            <- start from here
	root := t.TempDir()
	rootBeats := filepath.Join(root, ".beats")
	emptyBeats := filepath.Join(root, "subdir", ".beats")
	nestedDir := filepath.Join(root, "subdir", "nested")

	if err := os.MkdirAll(rootBeats, 0755); err != nil {
		t.Fatalf("Failed to create root .beats dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootBeats, "beats.jsonl"), []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create root beats.jsonl: %v", err)
	}
	if err := os.MkdirAll(emptyBeats, 0755); err != nil {
		t.Fatalf("Failed to create empty .beats dir: %v", err)
	}
	// Note: NOT creating beats.jsonl in emptyBeats
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	found := findBeatsDir(nestedDir)
	if found != rootBeats {
		t.Errorf("findBeatsDir() = %q, want %q (should skip empty .beats)", found, rootBeats)
	}
}

func TestGetBeatsDir_RespectsEnvVar(t *testing.T) {
	customDir := t.TempDir()

	// Set env var
	oldVal := os.Getenv(BeatsDirEnvVar)
	os.Setenv(BeatsDirEnvVar, customDir)
	defer os.Setenv(BeatsDirEnvVar, oldVal)

	dir, err := GetBeatsDir()
	if err != nil {
		t.Fatalf("GetBeatsDir() error = %v", err)
	}

	if dir != customDir {
		t.Errorf("GetBeatsDir() = %q, want %q (from env)", dir, customDir)
	}
}

func TestGetBeatsDir_EnvVarTakesPrecedence(t *testing.T) {
	// Create a .beats in cwd that should be ignored
	root := t.TempDir()
	beatsDir := filepath.Join(root, ".beats")
	customDir := filepath.Join(root, "custom-beats")

	if err := os.MkdirAll(beatsDir, 0755); err != nil {
		t.Fatalf("Failed to create .beats dir: %v", err)
	}
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatalf("Failed to create custom dir: %v", err)
	}

	// Change to root and set env var
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)

	oldVal := os.Getenv(BeatsDirEnvVar)
	os.Setenv(BeatsDirEnvVar, customDir)
	defer os.Setenv(BeatsDirEnvVar, oldVal)

	dir, err := GetBeatsDir()
	if err != nil {
		t.Fatalf("GetBeatsDir() error = %v", err)
	}

	if dir != customDir {
		t.Errorf("GetBeatsDir() = %q, want %q (env takes precedence)", dir, customDir)
	}
}
