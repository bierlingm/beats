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
