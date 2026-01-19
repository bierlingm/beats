package beat

import (
	"fmt"
	"time"
)

// Beat is a minimally structured, AI-indexable narrative unit.
// It captures insights, reflections, discoveries, and conceptual fragments
// that serve as upstream material for actionable work items (beads).
type Beat struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Impetus     Impetus     `json:"impetus"`
	Content     string      `json:"content"`
	References  []Reference `json:"references,omitempty"`
	Entities    []Entity    `json:"entities,omitempty"`
	LinkedBeads []string    `json:"linked_beads,omitempty"`
	SessionID   string      `json:"session_id,omitempty"`
	Context     *Context    `json:"context,omitempty"`
}

// Context captures the WALD directory context where the beat was captured.
// Used by Thermal WALD for beat-to-directory matching.
type Context struct {
	CapturePath     string  `json:"capture_path"`               // Absolute path where beat was captured (pwd)
	WALDDirectory   string  `json:"wald_directory,omitempty"`   // Resolved WALD directory path (relative to werk root)
	InferenceMethod string  `json:"inference_method,omitempty"` // How context was determined: capture_location, session_workspace, semantic, manual
	Confidence      float64 `json:"confidence,omitempty"`       // Confidence score 0-1
}

// Impetus captures the origin/motivation for recording a beat.
type Impetus struct {
	Label string            `json:"label"`
	Raw   string            `json:"raw,omitempty"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// Reference is an external resource linked to the beat.
type Reference struct {
	Kind    string            `json:"kind"`
	Subtype string            `json:"subtype,omitempty"`
	Locator string            `json:"locator"`
	Label   string            `json:"label,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// Entity is a named concept, person, or thing mentioned in the beat.
type Entity struct {
	Label    string            `json:"label"`
	Category string            `json:"category"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// NewBeat creates a new Beat with auto-generated ID and timestamps.
func NewBeat(content string, impetus Impetus) *Beat {
	now := time.Now().UTC()
	return &Beat{
		ID:          GenerateID(now),
		CreatedAt:   now,
		UpdatedAt:   now,
		Impetus:     impetus,
		Content:     content,
		References:  []Reference{},
		Entities:    []Entity{},
		LinkedBeads: []string{},
	}
}

// GenerateID creates a beat ID in the format: beat-YYYYMMDD-NNN
// The NNN suffix should be unique within the day; caller must ensure uniqueness.
func GenerateID(t time.Time) string {
	return fmt.Sprintf("beat-%s-%03d", t.Format("20060102"), 1)
}

// GenerateIDWithSequence creates a beat ID with a specific sequence number.
func GenerateIDWithSequence(t time.Time, seq int) string {
	return fmt.Sprintf("beat-%s-%03d", t.Format("20060102"), seq)
}

// ProposedBeat is a beat without ID/timestamps, used for robot-commit-beat input.
type ProposedBeat struct {
	Content     string      `json:"content"`
	Impetus     Impetus     `json:"impetus"`
	References  []Reference `json:"references,omitempty"`
	Entities    []Entity    `json:"entities,omitempty"`
	LinkedBeads []string    `json:"linked_beads,omitempty"`
	CreatedAt   *time.Time  `json:"created_at,omitempty"`
}

// ToBeat converts a ProposedBeat to a full Beat with ID and timestamps.
func (p *ProposedBeat) ToBeat(seq int) *Beat {
	t := time.Now().UTC()
	if p.CreatedAt != nil {
		t = p.CreatedAt.UTC()
	}
	return &Beat{
		ID:          GenerateIDWithSequence(t, seq),
		CreatedAt:   t,
		UpdatedAt:   t,
		Impetus:     p.Impetus,
		Content:     p.Content,
		References:  p.References,
		Entities:    p.Entities,
		LinkedBeads: p.LinkedBeads,
	}
}

// SearchResult represents a beat in search results with relevance score.
type SearchResult struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
	Impetus Impetus `json:"impetus"`
}

// BriefOutput is the output of --robot-brief.
type BriefOutput struct {
	BeatsUsed []string `json:"beats_used"`
	Outline   []string `json:"outline"`
}

// ContextForBeadOutput is the output of --robot-context-for-bead.
type ContextForBeadOutput struct {
	BeadID    string `json:"bead_id"`
	SeedBeats []Beat `json:"seed_beats"`
}

// ProposedEpic represents a suggested new epic derived from beats.
type ProposedEpic struct {
	Title      string   `json:"title"`
	SeedBeats  []string `json:"seed_beats"`
	Confidence float64  `json:"confidence"`
}

// ProposedLink represents a suggested link between beats and an existing bead.
type ProposedLink struct {
	BeadID     string   `json:"bead_id"`
	SeedBeats  []string `json:"seed_beats"`
	Reason     string   `json:"reason"`
	Confidence float64  `json:"confidence"`
}

// MapBeatsToBeadsOutput is the output of --robot-map-beats-to-beads.
type MapBeatsToBeadsOutput struct {
	ProposedNewEpics        []ProposedEpic `json:"proposed_new_epics"`
	ProposedLinksToExisting []ProposedLink `json:"proposed_links_to_existing"`
}

// DiffOutput is the output of --robot-diff.
type DiffOutput struct {
	NewBeats           []Beat   `json:"new_beats"`
	ModifiedBeats      []Beat   `json:"modified_beats"`
	BeatsLinkedToBeads []Beat   `json:"beats_linked_to_beads"`
	DeletedIDs         []string `json:"deleted_ids"`
}
