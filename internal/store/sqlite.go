package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
	_ "modernc.org/sqlite"
)

const DefaultDBFile = "beats.db"

// SQLiteStore provides SQLite-backed indexing over beats.
// The JSONL file remains the canonical store; SQLite is a derived index.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
	jsonl  *JSONLStore
}

// NewSQLiteStore creates a new SQLite store that indexes the given JSONL store.
func NewSQLiteStore(jsonl *JSONLStore) (*SQLiteStore, error) {
	dbPath := filepath.Join(jsonl.Dir(), DefaultDBFile)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	s := &SQLiteStore{
		db:     db,
		dbPath: dbPath,
		jsonl:  jsonl,
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS beats (
		id TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		content TEXT NOT NULL,
		impetus_label TEXT NOT NULL,
		impetus_raw TEXT,
		impetus_meta TEXT,
		references_json TEXT,
		entities_json TEXT,
		linked_beads_json TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_beats_created_at ON beats(created_at);
	CREATE INDEX IF NOT EXISTS idx_beats_updated_at ON beats(updated_at);
	CREATE INDEX IF NOT EXISTS idx_beats_impetus_label ON beats(impetus_label);

	CREATE VIRTUAL TABLE IF NOT EXISTS beats_fts USING fts5(
		id,
		content,
		impetus_label,
		impetus_raw,
		entities_text,
		content='beats',
		content_rowid='rowid'
	);

	CREATE TRIGGER IF NOT EXISTS beats_ai AFTER INSERT ON beats BEGIN
		INSERT INTO beats_fts(rowid, id, content, impetus_label, impetus_raw, entities_text)
		VALUES (new.rowid, new.id, new.content, new.impetus_label, new.impetus_raw, '');
	END;

	CREATE TRIGGER IF NOT EXISTS beats_ad AFTER DELETE ON beats BEGIN
		INSERT INTO beats_fts(beats_fts, rowid, id, content, impetus_label, impetus_raw, entities_text)
		VALUES ('delete', old.rowid, old.id, old.content, old.impetus_label, old.impetus_raw, '');
	END;

	CREATE TRIGGER IF NOT EXISTS beats_au AFTER UPDATE ON beats BEGIN
		INSERT INTO beats_fts(beats_fts, rowid, id, content, impetus_label, impetus_raw, entities_text)
		VALUES ('delete', old.rowid, old.id, old.content, old.impetus_label, old.impetus_raw, '');
		INSERT INTO beats_fts(rowid, id, content, impetus_label, impetus_raw, entities_text)
		VALUES (new.rowid, new.id, new.content, new.impetus_label, new.impetus_raw, '');
	END;

	CREATE TABLE IF NOT EXISTS sync_state (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Sync rebuilds the SQLite index from the JSONL file.
func (s *SQLiteStore) Sync() error {
	beats, err := s.jsonl.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read jsonl: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing data
	if _, err := tx.Exec("DELETE FROM beats"); err != nil {
		return err
	}

	// Insert all beats
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO beats 
		(id, created_at, updated_at, content, impetus_label, impetus_raw, impetus_meta, references_json, entities_json, linked_beads_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range beats {
		metaJSON, _ := json.Marshal(b.Impetus.Meta)
		refsJSON, _ := json.Marshal(b.References)
		entitiesJSON, _ := json.Marshal(b.Entities)
		linkedJSON, _ := json.Marshal(b.LinkedBeads)

		_, err := stmt.Exec(
			b.ID,
			b.CreatedAt.Format(time.RFC3339),
			b.UpdatedAt.Format(time.RFC3339),
			b.Content,
			b.Impetus.Label,
			b.Impetus.Raw,
			string(metaJSON),
			string(refsJSON),
			string(entitiesJSON),
			string(linkedJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert beat %s: %w", b.ID, err)
		}
	}

	// Update sync timestamp
	if _, err := tx.Exec(`INSERT OR REPLACE INTO sync_state (key, value) VALUES ('last_sync', ?)`,
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	return tx.Commit()
}

// SyncIfNeeded checks if the JSONL file has been modified and syncs if necessary.
func (s *SQLiteStore) SyncIfNeeded() error {
	jsonlPath := s.jsonl.Path()
	info, err := os.Stat(jsonlPath)
	if os.IsNotExist(err) {
		return nil // No JSONL file yet
	}
	if err != nil {
		return err
	}

	var lastSync string
	err = s.db.QueryRow("SELECT value FROM sync_state WHERE key = 'last_sync'").Scan(&lastSync)
	if err == sql.ErrNoRows {
		return s.Sync()
	}
	if err != nil {
		return err
	}

	lastSyncTime, err := time.Parse(time.RFC3339, lastSync)
	if err != nil {
		return s.Sync()
	}

	if info.ModTime().After(lastSyncTime) {
		return s.Sync()
	}

	return nil
}

// Search performs full-text search using SQLite FTS5.
func (s *SQLiteStore) Search(query string, maxResults int) ([]beat.SearchResult, error) {
	if err := s.SyncIfNeeded(); err != nil {
		return nil, err
	}

	// Escape special FTS5 characters and prepare query
	query = strings.TrimSpace(query)
	if query == "" {
		return []beat.SearchResult{}, nil
	}

	// Use simple contains match for now
	rows, err := s.db.Query(`
		SELECT b.id, b.content, b.impetus_label, b.impetus_raw, b.impetus_meta,
			   bm25(beats_fts) as score
		FROM beats_fts f
		JOIN beats b ON f.id = b.id
		WHERE beats_fts MATCH ?
		ORDER BY score
		LIMIT ?
	`, query+"*", maxResults)
	if err != nil {
		// Fallback to simple LIKE if FTS fails
		return s.searchLike(query, maxResults)
	}
	defer rows.Close()

	var results []beat.SearchResult
	for rows.Next() {
		var id, content, label, raw, metaJSON string
		var score float64
		if err := rows.Scan(&id, &content, &label, &raw, &metaJSON, &score); err != nil {
			continue
		}

		meta := make(map[string]string)
		json.Unmarshal([]byte(metaJSON), &meta)

		results = append(results, beat.SearchResult{
			ID:      id,
			Score:   -score, // bm25 returns negative scores, lower is better
			Content: content,
			Impetus: beat.Impetus{Label: label, Raw: raw, Meta: meta},
		})
	}

	return results, nil
}

func (s *SQLiteStore) searchLike(query string, maxResults int) ([]beat.SearchResult, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(`
		SELECT id, content, impetus_label, impetus_raw, impetus_meta
		FROM beats
		WHERE content LIKE ? OR impetus_label LIKE ? OR impetus_raw LIKE ?
		LIMIT ?
	`, pattern, pattern, pattern, maxResults)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []beat.SearchResult
	for rows.Next() {
		var id, content, label string
		var raw, metaJSON sql.NullString
		if err := rows.Scan(&id, &content, &label, &raw, &metaJSON); err != nil {
			continue
		}

		meta := make(map[string]string)
		if metaJSON.Valid {
			json.Unmarshal([]byte(metaJSON.String), &meta)
		}

		score := 0.5
		if strings.Contains(strings.ToLower(content), strings.ToLower(query)) {
			score += 0.25
		}
		if strings.Contains(strings.ToLower(label), strings.ToLower(query)) {
			score += 0.25
		}

		results = append(results, beat.SearchResult{
			ID:      id,
			Score:   score,
			Content: content,
			Impetus: beat.Impetus{Label: label, Raw: raw.String, Meta: meta},
		})
	}

	return results, nil
}

// Get retrieves a beat by ID from SQLite.
func (s *SQLiteStore) Get(id string) (*beat.Beat, error) {
	if err := s.SyncIfNeeded(); err != nil {
		return nil, err
	}

	var b beat.Beat
	var createdAt, updatedAt string
	var raw, metaJSON, refsJSON, entitiesJSON, linkedJSON sql.NullString

	err := s.db.QueryRow(`
		SELECT id, created_at, updated_at, content, impetus_label, impetus_raw, 
		       impetus_meta, references_json, entities_json, linked_beads_json
		FROM beats WHERE id = ?
	`, id).Scan(&b.ID, &createdAt, &updatedAt, &b.Content, &b.Impetus.Label,
		&raw, &metaJSON, &refsJSON, &entitiesJSON, &linkedJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("beat not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	b.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	b.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	b.Impetus.Raw = raw.String

	if metaJSON.Valid {
		json.Unmarshal([]byte(metaJSON.String), &b.Impetus.Meta)
	}
	if refsJSON.Valid {
		json.Unmarshal([]byte(refsJSON.String), &b.References)
	}
	if entitiesJSON.Valid {
		json.Unmarshal([]byte(entitiesJSON.String), &b.Entities)
	}
	if linkedJSON.Valid {
		json.Unmarshal([]byte(linkedJSON.String), &b.LinkedBeads)
	}

	return &b, nil
}

// GetSince returns beats created/modified since the given time.
func (s *SQLiteStore) GetSince(since time.Time) (new, modified, linked []beat.Beat, err error) {
	if err := s.SyncIfNeeded(); err != nil {
		return nil, nil, nil, err
	}

	sinceStr := since.Format(time.RFC3339)

	// New beats
	newRows, err := s.db.Query(`
		SELECT id, created_at, updated_at, content, impetus_label, impetus_raw,
		       impetus_meta, references_json, entities_json, linked_beads_json
		FROM beats WHERE created_at >= ?
	`, sinceStr)
	if err != nil {
		return nil, nil, nil, err
	}
	new, err = scanBeats(newRows)
	if err != nil {
		return nil, nil, nil, err
	}

	// Modified beats (updated after created, and updated since)
	modRows, err := s.db.Query(`
		SELECT id, created_at, updated_at, content, impetus_label, impetus_raw,
		       impetus_meta, references_json, entities_json, linked_beads_json
		FROM beats WHERE updated_at >= ? AND created_at < ?
	`, sinceStr, sinceStr)
	if err != nil {
		return nil, nil, nil, err
	}
	modified, err = scanBeats(modRows)
	if err != nil {
		return nil, nil, nil, err
	}

	// Beats with linked beads that were updated since
	linkedRows, err := s.db.Query(`
		SELECT id, created_at, updated_at, content, impetus_label, impetus_raw,
		       impetus_meta, references_json, entities_json, linked_beads_json
		FROM beats WHERE linked_beads_json != '[]' AND linked_beads_json != 'null' 
		AND linked_beads_json IS NOT NULL AND updated_at >= ?
	`, sinceStr)
	if err != nil {
		return nil, nil, nil, err
	}
	linked, err = scanBeats(linkedRows)
	if err != nil {
		return nil, nil, nil, err
	}

	return new, modified, linked, nil
}

func scanBeats(rows *sql.Rows) ([]beat.Beat, error) {
	defer rows.Close()
	var beats []beat.Beat

	for rows.Next() {
		var b beat.Beat
		var createdAt, updatedAt string
		var raw, metaJSON, refsJSON, entitiesJSON, linkedJSON sql.NullString

		if err := rows.Scan(&b.ID, &createdAt, &updatedAt, &b.Content, &b.Impetus.Label,
			&raw, &metaJSON, &refsJSON, &entitiesJSON, &linkedJSON); err != nil {
			continue
		}

		b.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		b.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		b.Impetus.Raw = raw.String

		if metaJSON.Valid {
			json.Unmarshal([]byte(metaJSON.String), &b.Impetus.Meta)
		}
		if refsJSON.Valid {
			json.Unmarshal([]byte(refsJSON.String), &b.References)
		}
		if entitiesJSON.Valid {
			json.Unmarshal([]byte(entitiesJSON.String), &b.Entities)
		}
		if linkedJSON.Valid {
			json.Unmarshal([]byte(linkedJSON.String), &b.LinkedBeads)
		}

		beats = append(beats, b)
	}

	return beats, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Path returns the database file path.
func (s *SQLiteStore) Path() string {
	return s.dbPath
}
