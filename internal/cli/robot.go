package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/moritzbierling/beats/internal/beat"
	"github.com/moritzbierling/beats/internal/store"
)

// RobotCLI handles robot-facing CLI commands (JSON in/out).
type RobotCLI struct {
	store *store.JSONLStore
}

// NewRobotCLI creates a new RobotCLI.
func NewRobotCLI(s *store.JSONLStore) *RobotCLI {
	return &RobotCLI{store: s}
}

// Help outputs JSON describing all robot commands.
func (c *RobotCLI) Help() error {
	help := map[string]interface{}{
		"version": "0.1.1",
		"commands": []map[string]interface{}{
			{
				"name":        "--robot-help",
				"description": "Output JSON describing all robot commands and their input/output schemas",
				"input":       nil,
				"output":      "this schema",
			},
			{
				"name":        "--robot-propose-beat",
				"description": "Propose a structured beat from raw text (AI extracts entities, references, etc.)",
				"input": map[string]interface{}{
					"raw_text":     "string (required) - raw text to extract beat from",
					"impetus_hint": "string (optional) - short phrase about why recording this",
					"context": map[string]string{
						"channel":      "coaching|web|journal|other",
						"counterparty": "name of person involved",
						"session_id":   "unique session identifier",
					},
				},
				"output": map[string]interface{}{
					"proposed_beat": "Beat object without id/timestamps",
					"alternatives":  "array of alternative Beat proposals",
				},
			},
			{
				"name":        "--robot-commit-beat",
				"description": "Commit a proposed beat to storage, assigning ID and timestamps",
				"input": map[string]interface{}{
					"content":      "string (required) - the beat content",
					"impetus":      "Impetus object (required)",
					"references":   "array of Reference objects (optional)",
					"entities":     "array of Entity objects (optional)",
					"linked_beads": "array of bead IDs (optional)",
				},
				"output": "Beat object with id and timestamps",
			},
			{
				"name":        "--robot-search",
				"description": "Search beats by keyword/semantic query",
				"input": map[string]interface{}{
					"query":       "string (required) - search query",
					"max_results": "int (optional, default 20)",
				},
				"output": map[string]interface{}{
					"results": "array of {id, score, content, impetus}",
				},
			},
			{
				"name":        "--robot-brief",
				"description": "Generate a thematic brief from relevant beats",
				"input": map[string]interface{}{
					"topic":     "string (required) - topic to brief on",
					"audience":  "string (LLM|human)",
					"max_beats": "int (optional, default 30)",
				},
				"output": map[string]interface{}{
					"beats_used": "array of beat IDs",
					"outline":    "array of outline strings",
				},
			},
			{
				"name":        "--robot-context-for-bead",
				"description": "Get narrative context (beats) for a specific bead",
				"input": map[string]interface{}{
					"bead_id": "string (required) - the bead ID to get context for",
				},
				"output": map[string]interface{}{
					"bead_id":    "string",
					"seed_beats": "array of Beat objects",
				},
			},
			{
				"name":        "--robot-map-beats-to-beads",
				"description": "Suggest how beats might map to epics/beads",
				"input": map[string]interface{}{
					"beat_ids": "array of beat IDs to analyze",
				},
				"output": map[string]interface{}{
					"proposed_new_epics":        "array of {title, seed_beats, confidence}",
					"proposed_links_to_existing": "array of {bead_id, seed_beats, reason, confidence}",
				},
			},
			{
				"name":        "--robot-diff",
				"description": "Get changes since a given timestamp",
				"input": map[string]interface{}{
					"diff_since": "RFC3339 timestamp",
				},
				"output": map[string]interface{}{
					"new_beats":            "array of new Beat objects",
					"modified_beats":       "array of modified Beat objects",
					"beats_linked_to_beads": "array of Beat objects with new links",
					"deleted_ids":          "array of deleted beat IDs",
				},
			},
		},
		"schemas": map[string]interface{}{
			"Beat": map[string]string{
				"id":           "beat-YYYYMMDD-NNN",
				"created_at":   "RFC3339 timestamp",
				"updated_at":   "RFC3339 timestamp",
				"impetus":      "Impetus object",
				"content":      "string",
				"references":   "array of Reference",
				"entities":     "array of Entity",
				"linked_beads": "array of bead IDs",
			},
			"Impetus": map[string]string{
				"label": "string - human-readable label",
				"raw":   "string - raw source reference",
				"meta":  "object - additional metadata",
			},
			"Reference": map[string]string{
				"kind":    "url|file|etc",
				"subtype": "github|web|pdf|etc",
				"locator": "URL or path",
				"label":   "human-readable label",
				"meta":    "object",
			},
			"Entity": map[string]string{
				"label":    "entity name",
				"category": "person|concept|tool|etc",
				"meta":     "object",
			},
		},
	}

	return outputJSON(help)
}

// ProposeBeatInput is the input for --robot-propose-beat.
type ProposeBeatInput struct {
	RawText     string `json:"raw_text"`
	ImpetusHint string `json:"impetus_hint,omitempty"`
	Context     struct {
		Channel      string `json:"channel,omitempty"`
		Counterparty string `json:"counterparty,omitempty"`
		SessionID    string `json:"session_id,omitempty"`
	} `json:"context,omitempty"`
}

// ProposeBeatOutput is the output for --robot-propose-beat.
type ProposeBeatOutput struct {
	ProposedBeat beat.ProposedBeat   `json:"proposed_beat"`
	Alternatives []beat.ProposedBeat `json:"alternatives"`
}

// ProposeBeat proposes a structured beat from raw text.
// NOTE: This is a stub - actual AI extraction would happen externally.
func (c *RobotCLI) ProposeBeat(input io.Reader) error {
	var in ProposeBeatInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if in.RawText == "" {
		return outputError("raw_text is required", nil)
	}

	impetusLabel := in.ImpetusHint
	if impetusLabel == "" {
		impetusLabel = "Extracted from raw input"
	}

	meta := make(map[string]string)
	if in.Context.Channel != "" {
		meta["channel"] = in.Context.Channel
	}
	if in.Context.Counterparty != "" {
		meta["counterparty"] = in.Context.Counterparty
	}
	if in.Context.SessionID != "" {
		meta["session_id"] = in.Context.SessionID
	}

	proposed := beat.ProposedBeat{
		Content: in.RawText,
		Impetus: beat.Impetus{
			Label: impetusLabel,
			Raw:   truncate(in.RawText, 100),
			Meta:  meta,
		},
		References:  []beat.Reference{},
		Entities:    []beat.Entity{},
		LinkedBeads: []string{},
	}

	output := ProposeBeatOutput{
		ProposedBeat: proposed,
		Alternatives: []beat.ProposedBeat{},
	}

	return outputJSON(output)
}

// CommitBeat commits a proposed beat to storage.
func (c *RobotCLI) CommitBeat(input io.Reader) error {
	var proposed beat.ProposedBeat
	if err := json.NewDecoder(input).Decode(&proposed); err != nil {
		return outputError("invalid input JSON", err)
	}

	if proposed.Content == "" {
		return outputError("content is required", nil)
	}

	seq, err := c.store.NextSequence()
	if err != nil {
		return outputError("failed to get sequence", err)
	}

	b := proposed.ToBeat(seq)

	if err := c.store.Append(b); err != nil {
		return outputError("failed to save beat", err)
	}

	return outputJSON(b)
}

// SearchInput is the input for --robot-search.
type SearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

// SearchOutput is the output for --robot-search.
type SearchOutput struct {
	Results []beat.SearchResult `json:"results"`
}

// Search performs a search and returns JSON results.
func (c *RobotCLI) Search(input io.Reader) error {
	var in SearchInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if in.Query == "" {
		return outputError("query is required", nil)
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 20
	}

	results, err := c.store.Search(in.Query, maxResults)
	if err != nil {
		return outputError("search failed", err)
	}

	return outputJSON(SearchOutput{Results: results})
}

// BriefInput is the input for --robot-brief.
type BriefInput struct {
	Topic    string `json:"topic"`
	Audience string `json:"audience,omitempty"`
	MaxBeats int    `json:"max_beats,omitempty"`
}

// Brief generates a thematic brief from relevant beats.
// NOTE: This is a stub - actual AI synthesis would happen externally.
func (c *RobotCLI) Brief(input io.Reader) error {
	var in BriefInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if in.Topic == "" {
		return outputError("topic is required", nil)
	}

	maxBeats := in.MaxBeats
	if maxBeats <= 0 {
		maxBeats = 30
	}

	results, err := c.store.Search(in.Topic, maxBeats)
	if err != nil {
		return outputError("search failed", err)
	}

	beatsUsed := make([]string, len(results))
	for i, r := range results {
		beatsUsed[i] = r.ID
	}

	output := beat.BriefOutput{
		BeatsUsed: beatsUsed,
		Outline: []string{
			fmt.Sprintf("Topic: %s", in.Topic),
			fmt.Sprintf("Found %d relevant beats", len(results)),
			"[AI synthesis would generate detailed outline here]",
		},
	}

	return outputJSON(output)
}

// ContextForBeadInput is the input for --robot-context-for-bead.
type ContextForBeadInput struct {
	BeadID string `json:"bead_id"`
}

// ContextForBead returns narrative context for a bead.
func (c *RobotCLI) ContextForBead(input io.Reader) error {
	var in ContextForBeadInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if in.BeadID == "" {
		return outputError("bead_id is required", nil)
	}

	beats, err := c.store.GetByLinkedBead(in.BeadID)
	if err != nil {
		return outputError("failed to get linked beats", err)
	}

	output := beat.ContextForBeadOutput{
		BeadID:    in.BeadID,
		SeedBeats: beats,
	}

	return outputJSON(output)
}

// MapBeatsToBeadsInput is the input for --robot-map-beats-to-beads.
type MapBeatsToBeadsInput struct {
	BeatIDs []string `json:"beat_ids"`
}

// MapBeatsToBeads suggests how beats might map to epics/beads.
// NOTE: This is a stub - actual AI mapping would happen externally.
func (c *RobotCLI) MapBeatsToBeads(input io.Reader) error {
	var in MapBeatsToBeadsInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if len(in.BeatIDs) == 0 {
		return outputError("beat_ids is required", nil)
	}

	output := beat.MapBeatsToBeadsOutput{
		ProposedNewEpics: []beat.ProposedEpic{
			{
				Title:      "[AI would suggest epic title based on beat content]",
				SeedBeats:  in.BeatIDs,
				Confidence: 0.0,
			},
		},
		ProposedLinksToExisting: []beat.ProposedLink{},
	}

	return outputJSON(output)
}

// DiffInput is the input for --robot-diff.
type DiffInput struct {
	DiffSince string `json:"diff_since"`
}

// Diff returns changes since a given timestamp.
func (c *RobotCLI) Diff(input io.Reader) error {
	var in DiffInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	since, err := time.Parse(time.RFC3339, in.DiffSince)
	if err != nil {
		return outputError("invalid diff_since timestamp (use RFC3339)", err)
	}

	newBeats, modified, linked, err := c.store.GetSince(since)
	if err != nil {
		return outputError("failed to get beats", err)
	}

	output := beat.DiffOutput{
		NewBeats:           newBeats,
		ModifiedBeats:      modified,
		BeatsLinkedToBeads: linked,
		DeletedIDs:         []string{},
	}

	return outputJSON(output)
}

func outputJSON(v interface{}) error {
	enc := json.NewEncoder(jsonOutput)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func outputError(msg string, err error) error {
	errObj := map[string]interface{}{
		"error": msg,
	}
	if err != nil {
		errObj["details"] = err.Error()
	}
	return outputJSON(errObj)
}

// jsonOutput is where JSON output is written (defaults to stdout).
var jsonOutput io.Writer = nil

// SetJSONOutput sets the output writer for JSON responses.
func SetJSONOutput(w io.Writer) {
	jsonOutput = w
}
