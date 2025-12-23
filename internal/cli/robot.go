package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
	"github.com/bierlingm/beats/internal/hooks"
	"github.com/bierlingm/beats/internal/store"
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
					"created_at":   "RFC3339 timestamp (optional) - backdate the beat",
				},
				"output": "Beat object with id and timestamps",
			},
			{
				"name":        "--robot-search",
				"description": "Search beats by keyword or semantic query",
				"input": map[string]interface{}{
					"query":       "string (required) - search query",
					"max_results": "int (optional, default 20)",
					"semantic":    "bool (optional, default false) - use osgrep semantic search instead of keyword FTS5",
				},
				"output": map[string]interface{}{
					"results":  "array of {id, score, content, impetus}",
					"mode":     "string - 'keyword' or 'semantic'",
					"fallback": "bool - true if semantic was requested but fell back to keyword",
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
					"proposed_new_epics":         "array of {title, seed_beats, confidence}",
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
					"new_beats":             "array of new Beat objects",
					"modified_beats":        "array of modified Beat objects",
					"beats_linked_to_beads": "array of Beat objects with new links",
					"deleted_ids":           "array of deleted beat IDs",
				},
			},
			{
				"name":        "--robot-link-beat",
				"description": "Link a beat to one or more beads (adds to existing links)",
				"input": map[string]interface{}{
					"beat_id":  "string (required) - the beat ID to update",
					"bead_ids": "array of strings (required) - bead IDs to link",
				},
				"output": "Beat object with updated linked_beads",
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
	ProposedBeat     beat.ProposedBeat   `json:"proposed_beat"`
	ExtractedURLs    []string            `json:"extracted_urls"`
	ExtractionPrompt string              `json:"extraction_prompt"`
	Alternatives     []beat.ProposedBeat `json:"alternatives"`
}

// ProposeBeat proposes a structured beat from raw text.
// Extracts URLs and provides a prompt for LLM to do richer extraction.
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

	// Extract URLs from raw text
	urls := extractURLs(in.RawText)

	// Build references from extracted URLs
	var refs []beat.Reference
	for _, url := range urls {
		refs = append(refs, beat.Reference{
			Kind:    "url",
			Subtype: classifyURL(url),
			Locator: url,
			Label:   "",
		})
	}

	proposed := beat.ProposedBeat{
		Content: in.RawText,
		Impetus: beat.Impetus{
			Label: impetusLabel,
			Raw:   truncate(in.RawText, 100),
			Meta:  meta,
		},
		References:  refs,
		Entities:    []beat.Entity{},
		LinkedBeads: []string{},
	}

	prompt := fmt.Sprintf(`Extract structured information from this beat:

RAW TEXT:
%s

CONTEXT:
- Channel: %s
- Counterparty: %s
- Impetus hint: %s

EXTRACT:
1. ENTITIES: People, concepts, tools, projects mentioned (format: {"label": "name", "category": "person|concept|tool|project"})
2. REFERENCES: Any URLs, file paths, or external resources (already extracted: %d URLs)
3. IMPETUS LABEL: A concise (3-7 word) label describing why this was recorded
4. CONTENT: Clean up the content if needed (fix typos, improve clarity) while preserving meaning
5. LINKED BEADS: If this relates to known beads, suggest IDs to link

Return a JSON object matching the ProposedBeat schema.`,
		in.RawText,
		in.Context.Channel,
		in.Context.Counterparty,
		in.ImpetusHint,
		len(urls),
	)

	output := ProposeBeatOutput{
		ProposedBeat:     proposed,
		ExtractedURLs:    urls,
		ExtractionPrompt: prompt,
		Alternatives:     []beat.ProposedBeat{},
	}

	return outputJSON(output)
}

// extractURLs finds URLs in text using a simple pattern.
func extractURLs(text string) []string {
	var urls []string
	words := strings.Fields(text)
	for _, word := range words {
		// Clean punctuation from end
		word = strings.TrimRight(word, ".,;:!?)")
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			urls = append(urls, word)
		}
	}
	return urls
}

// classifyURL returns a subtype based on the URL domain.
func classifyURL(url string) string {
	switch {
	case strings.Contains(url, "github.com"):
		return "github"
	case strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be"):
		return "youtube"
	case strings.Contains(url, "twitter.com") || strings.Contains(url, "x.com"):
		return "twitter"
	case strings.Contains(url, ".pdf"):
		return "pdf"
	default:
		return "web"
	}
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
	Semantic   bool   `json:"semantic,omitempty"`
}

// SearchOutput is the output for --robot-search.
type SearchOutput struct {
	Results  []beat.SearchResult `json:"results"`
	Mode     string              `json:"mode,omitempty"`
	Fallback bool                `json:"fallback,omitempty"`
}

// Search performs a search and returns JSON results.
// When semantic=true, uses osgrep for semantic/embedding-based search.
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

	output, err := store.HybridSearch(c.store, in.Query, maxResults, in.Semantic)
	if err != nil {
		return outputError("search failed", err)
	}

	return outputJSON(SearchOutput{
		Results:  output.Results,
		Mode:     output.Mode,
		Fallback: output.Fallback,
	})
}

// BriefInput is the input for --robot-brief.
type BriefInput struct {
	Topic    string `json:"topic"`
	Audience string `json:"audience,omitempty"`
	MaxBeats int    `json:"max_beats,omitempty"`
}

// BriefOutput is the output for --robot-brief.
type BriefOutput struct {
	Topic       string      `json:"topic"`
	Audience    string      `json:"audience"`
	BeatsUsed   []string    `json:"beats_used"`
	BeatsData   []beat.Beat `json:"beats_data"`
	BriefPrompt string      `json:"brief_prompt"`
}

// Brief generates a thematic brief from relevant beats.
// Returns full beat data + synthesis prompt for LLM processing.
func (c *RobotCLI) Brief(input io.Reader) error {
	var in BriefInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if in.Topic == "" {
		return outputError("topic is required", nil)
	}

	audience := in.Audience
	if audience == "" {
		audience = "human"
	}

	maxBeats := in.MaxBeats
	if maxBeats <= 0 {
		maxBeats = 30
	}

	results, err := c.store.Search(in.Topic, maxBeats)
	if err != nil {
		return outputError("search failed", err)
	}

	// Get full beat data
	beatIDs := make([]string, len(results))
	for i, r := range results {
		beatIDs[i] = r.ID
	}
	beatsData, err := c.store.GetByIDs(beatIDs)
	if err != nil {
		return outputError("failed to get beats", err)
	}

	// Build beat summaries for prompt
	var beatSummaries []string
	for _, b := range beatsData {
		summary := fmt.Sprintf("- [%s] (%s) %s", b.ID, b.Impetus.Label, truncate(b.Content, 200))
		beatSummaries = append(beatSummaries, summary)
	}

	audienceGuidance := "Write for a human reader - clear, concise, actionable."
	if audience == "LLM" {
		audienceGuidance = "Write for an LLM agent - structured, machine-parseable, include metadata."
	}

	prompt := fmt.Sprintf(`Generate a thematic brief on: %s

RELEVANT BEATS (%d found):
%s

AUDIENCE: %s
%s

BRIEF STRUCTURE:
1. EXECUTIVE SUMMARY: 2-3 sentences capturing the core insight
2. KEY THEMES: Major patterns or clusters in this material
3. TIMELINE: How thinking evolved (if applicable)
4. OPEN QUESTIONS: Unresolved items or areas needing exploration
5. ACTION ITEMS: Concrete next steps that emerge from this material
6. CONNECTIONS: Links to other topics, beads, or external resources

Keep the brief focused and actionable. Cite beat IDs when referencing specific insights.`,
		in.Topic,
		len(beatsData),
		strings.Join(beatSummaries, "\n"),
		audience,
		audienceGuidance,
	)

	output := BriefOutput{
		Topic:       in.Topic,
		Audience:    audience,
		BeatsUsed:   beatIDs,
		BeatsData:   beatsData,
		BriefPrompt: prompt,
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
	BeatIDs       []string `json:"beat_ids"`
	ExistingBeads []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
	} `json:"existing_beads,omitempty"`
}

// MapBeatsToBeadsOutput is the output for --robot-map-beats-to-beads.
type MapBeatsToBeadsOutput struct {
	BeatsData     []beat.Beat `json:"beats_data"`
	MappingPrompt string      `json:"mapping_prompt"`
}

// MapBeatsToBeads suggests how beats might map to epics/beads.
// Returns beat data + mapping prompt for LLM processing.
func (c *RobotCLI) MapBeatsToBeads(input io.Reader) error {
	var in MapBeatsToBeadsInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	// If no beat IDs provided, use all beats
	var beatsData []beat.Beat
	var err error
	if len(in.BeatIDs) == 0 {
		beatsData, err = c.store.ReadAll()
		if err != nil {
			return outputError("failed to read beats", err)
		}
	} else {
		beatsData, err = c.store.GetByIDs(in.BeatIDs)
		if err != nil {
			return outputError("failed to get beats", err)
		}
	}

	// Build beat summaries
	var beatSummaries []string
	for _, b := range beatsData {
		linkedStr := ""
		if len(b.LinkedBeads) > 0 {
			linkedStr = fmt.Sprintf(" [already linked to: %s]", strings.Join(b.LinkedBeads, ", "))
		}
		summary := fmt.Sprintf("- [%s] (%s) %s%s", b.ID, b.Impetus.Label, truncate(b.Content, 150), linkedStr)
		beatSummaries = append(beatSummaries, summary)
	}

	// Build existing beads context
	var beadsSummaries []string
	for _, bead := range in.ExistingBeads {
		desc := bead.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		beadsSummaries = append(beadsSummaries, fmt.Sprintf("- [%s] %s: %s", bead.ID, bead.Title, desc))
	}

	existingBeadsSection := "No existing beads provided. Propose new epics only."
	if len(beadsSummaries) > 0 {
		existingBeadsSection = fmt.Sprintf("EXISTING BEADS (%d):\n%s", len(beadsSummaries), strings.Join(beadsSummaries, "\n"))
	}

	prompt := fmt.Sprintf(`Map these beats to actionable beads (epics/tasks).

BEATS TO ANALYZE (%d):
%s

%s

YOUR TASK:
1. CLUSTER ANALYSIS: Group beats by theme/topic
2. PROPOSED NEW EPICS: For each cluster, propose a new epic if none exists
   Format: {"title": "...", "description": "...", "seed_beats": ["beat-id-1", "beat-id-2"], "confidence": 0.0-1.0}
3. PROPOSED LINKS: Match beats to existing beads they should link to
   Format: {"beat_id": "...", "bead_id": "...", "reason": "...", "confidence": 0.0-1.0}
4. ORPHAN BEATS: Identify beats that don't fit any pattern (might be noise or need more context)

Return JSON with:
{
  "proposed_new_epics": [...],
  "proposed_links_to_existing": [...],
  "orphan_beats": [...],
  "clusters": [{"theme": "...", "beat_ids": [...]}]
}`,
		len(beatsData),
		strings.Join(beatSummaries, "\n"),
		existingBeadsSection,
	)

	output := MapBeatsToBeadsOutput{
		BeatsData:     beatsData,
		MappingPrompt: prompt,
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

// LinkBeatInput is the input for --robot-link-beat.
type LinkBeatInput struct {
	BeatID  string   `json:"beat_id"`
	BeadIDs []string `json:"bead_ids"`
}

// LinkBeat links a beat to one or more beads.
func (c *RobotCLI) LinkBeat(input io.Reader) error {
	var in LinkBeatInput
	if err := json.NewDecoder(input).Decode(&in); err != nil {
		return outputError("invalid input JSON", err)
	}

	if in.BeatID == "" {
		return outputError("beat_id is required", nil)
	}
	if len(in.BeadIDs) == 0 {
		return outputError("bead_ids is required (at least one bead ID)", nil)
	}

	updated, err := c.store.Update(in.BeatID, func(b *beat.Beat) error {
		// Add new bead IDs, avoiding duplicates
		existing := make(map[string]bool)
		for _, id := range b.LinkedBeads {
			existing[id] = true
		}
		for _, id := range in.BeadIDs {
			if !existing[id] {
				b.LinkedBeads = append(b.LinkedBeads, id)
				existing[id] = true
			}
		}
		return nil
	})
	if err != nil {
		return outputError("failed to link beat", err)
	}

	return outputJSON(updated)
}

// SynthesisStatus returns the current synthesis request if one exists.
func (c *RobotCLI) SynthesisStatus() error {
	req, err := hooks.GetSynthesisRequest(c.store.Dir())
	if err != nil {
		return outputJSON(map[string]interface{}{
			"pending": false,
			"message": "No synthesis pending",
		})
	}

	return outputJSON(map[string]interface{}{
		"pending":          true,
		"triggered_at":     req.TriggeredAt,
		"beats_since_last": req.BeatsSinceLast,
		"total_beats":      req.TotalBeats,
		"recent_beats":     req.RecentBeats,
		"synthesis_prompt": req.SynthesisPrompt,
	})
}

// SynthesisClear clears the synthesis request file.
func (c *RobotCLI) SynthesisClear() error {
	if err := hooks.ClearSynthesisNeeded(c.store.Dir()); err != nil {
		return outputError("failed to clear synthesis", err)
	}

	return outputJSON(map[string]interface{}{
		"cleared": true,
		"message": "Synthesis request cleared",
	})
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
