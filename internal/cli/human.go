package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/bierlingm/beats/internal/beat"
	"github.com/bierlingm/beats/internal/capture"
	"github.com/bierlingm/beats/internal/embeddings"
	"github.com/bierlingm/beats/internal/entity"
	"github.com/bierlingm/beats/internal/impetus"
	"github.com/bierlingm/beats/internal/store"
)

// HumanCLI handles human-facing CLI commands.
type HumanCLI struct {
	store *store.JSONLStore
}

// NewHumanCLI creates a new HumanCLI.
func NewHumanCLI(s *store.JSONLStore) *HumanCLI {
	return &HumanCLI{store: s}
}

// AddOptions contains options for creating a beat.
type AddOptions struct {
	Content      string
	ImpetusLabel string
	WebURL       string
	GitHubRef    string
	TwitterURL   string
	Coaching     bool
	Session      bool
}

// Add creates a new beat with the given content.
func (c *HumanCLI) Add(content string, impetusLabel string) error {
	return c.AddWithOptions(AddOptions{
		Content:      content,
		ImpetusLabel: impetusLabel,
	})
}

// resolveWALDDirectory resolves a capture path to a WALD directory.
// Returns the relative WALD directory path and confidence score.
func resolveWALDDirectory(capturePath string) (string, float64) {
	// Find the werk root by looking for WALD.yaml
	werkRoot := ""
	dir := capturePath
	for {
		if _, err := os.Stat(filepath.Join(dir, "WALD.yaml")); err == nil {
			werkRoot = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if werkRoot == "" {
		return "", 0
	}

	// Get relative path from werk root
	relPath, err := filepath.Rel(werkRoot, capturePath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", 0
	}

	// Return the relative path as the WALD directory
	// The temperature engine will match this to actual WALD.yaml entries
	if relPath == "." {
		return "", 0
	}

	return relPath, 1.0
}

// AddWithOptions creates a new beat with extended options.
func (c *HumanCLI) AddWithOptions(opts AddOptions) error {
	var finalContent string
	var finalImpetus string

	// Handle web capture
	if opts.WebURL != "" {
		web, err := capture.CaptureFromURL(opts.WebURL, opts.Content)
		if err != nil {
			return fmt.Errorf("web capture failed: %w", err)
		}
		finalContent = web.Content
		finalImpetus = web.Impetus
	} else if opts.GitHubRef != "" {
		// Handle GitHub capture
		gh, err := capture.CaptureFromGitHub(opts.GitHubRef, opts.Content)
		if err != nil {
			return fmt.Errorf("GitHub capture failed: %w", err)
		}
		finalContent = gh.Content
		finalImpetus = "GitHub discovery"
	} else if opts.TwitterURL != "" {
		// Handle Twitter/X capture (basic URL capture)
		web, err := capture.CaptureFromURL(opts.TwitterURL, opts.Content)
		if err != nil {
			// Twitter often blocks, so just store the URL
			finalContent = fmt.Sprintf("X/Twitter post\n\nURL: %s", opts.TwitterURL)
			if opts.Content != "" {
				finalContent = fmt.Sprintf("%s\n\n%s", finalContent, opts.Content)
			}
		} else {
			finalContent = web.Content
		}
		finalImpetus = "X/Twitter capture"
	} else {
		finalContent = opts.Content
		finalImpetus = opts.ImpetusLabel
	}

	// Override impetus for special flags
	if opts.Coaching {
		finalImpetus = "Coaching insight"
	} else if opts.Session {
		finalImpetus = "Session insight"
	}

	seq, err := c.store.NextSequence()
	if err != nil {
		return fmt.Errorf("failed to get sequence: %w", err)
	}

	imp := beat.Impetus{
		Label: finalImpetus,
	}
	if finalImpetus == "" {
		if inferred := impetus.Infer(finalContent); inferred != "" {
			imp.Label = inferred
		} else {
			imp.Label = "Manual entry"
		}
	}

	// Extract entities from content using WALD.yaml data
	extractedEntities := entity.ExtractEntities(finalContent, "")

	b := &beat.Beat{
		ID:          beat.GenerateIDWithSequence(time.Now().UTC(), seq),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Impetus:     imp,
		Content:     finalContent,
		References:  []beat.Reference{},
		Entities:    extractedEntities,
		LinkedBeads: []string{},
	}

	if sessionID := os.Getenv("FACTORY_SESSION_ID"); sessionID != "" {
		b.SessionID = sessionID
	}

	// Global store architecture: beats have NO context field.
	// Context/directory assignment happens via claims at query time (P2).
	// b.Context is left nil.

	if err := c.store.Append(b); err != nil {
		return fmt.Errorf("failed to save beat: %w", err)
	}

	fmt.Printf("Created beat: %s\n", b.ID)
	return nil
}

// List displays all beats, optionally filtered by session.
func (c *HumanCLI) List(sessionFilter string) error {
	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
	}

	// Resolve "current" to actual session ID
	if sessionFilter == "current" {
		sessionFilter = os.Getenv("FACTORY_SESSION_ID")
	}

	// Filter by session if specified
	if sessionFilter != "" {
		var filtered []beat.Beat
		for _, b := range beats {
			if strings.HasPrefix(b.SessionID, sessionFilter) {
				filtered = append(filtered, b)
			}
		}
		beats = filtered
	}

	if len(beats) == 0 {
		fmt.Println("No beats found.")
		return nil
	}

	fmt.Printf("Found %d beat(s):\n\n", len(beats))
	for _, b := range beats {
		preview := truncate(b.Content, 60)
		fmt.Printf("  %s  %s\n", b.ID, b.Impetus.Label)
		fmt.Printf("            %s\n\n", preview)
	}

	return nil
}

// Show displays a single beat by ID.
func (c *HumanCLI) Show(id string) error {
	b, err := c.store.Get(id)
	if err != nil {
		return err
	}

	fmt.Printf("ID:         %s\n", b.ID)
	fmt.Printf("Created:    %s\n", b.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:    %s\n", b.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Impetus:    %s\n", b.Impetus.Label)
	if b.Impetus.Raw != "" {
		fmt.Printf("Raw:        %s\n", b.Impetus.Raw)
	}
	if len(b.Impetus.Meta) > 0 {
		fmt.Printf("Meta:       %v\n", b.Impetus.Meta)
	}
	fmt.Printf("\nContent:\n%s\n", b.Content)

	if len(b.References) > 0 {
		fmt.Printf("\nReferences:\n")
		for _, ref := range b.References {
			fmt.Printf("  - [%s] %s: %s\n", ref.Kind, ref.Label, ref.Locator)
		}
	}

	if len(b.Entities) > 0 {
		fmt.Printf("\nEntities:\n")
		for _, ent := range b.Entities {
			fmt.Printf("  - %s (%s)\n", ent.Label, ent.Category)
		}
	}

	if len(b.LinkedBeads) > 0 {
		fmt.Printf("\nLinked Beads:\n")
		for _, beadID := range b.LinkedBeads {
			fmt.Printf("  - %s\n", beadID)
		}
	}

	return nil
}

// Search finds beats matching the query, optionally filtered by session.
func (c *HumanCLI) Search(query string, maxResults int, sessionFilter string) error {
	if maxResults <= 0 {
		maxResults = 20
	}

	// Resolve "current" to actual session ID
	if sessionFilter == "current" {
		sessionFilter = os.Getenv("FACTORY_SESSION_ID")
	}

	// If session filter specified, we need to filter results
	if sessionFilter != "" {
		beats, err := c.store.ReadAll()
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		queryLower := strings.ToLower(query)
		var results []beat.SearchResult
		for _, b := range beats {
			// Check session filter
			if !strings.HasPrefix(b.SessionID, sessionFilter) {
				continue
			}

			contentLower := strings.ToLower(b.Content)
			labelLower := strings.ToLower(b.Impetus.Label)

			score := 0.0
			if strings.Contains(contentLower, queryLower) {
				score += 0.5
			}
			if strings.Contains(labelLower, queryLower) {
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

		if len(results) == 0 {
			fmt.Printf("No beats found matching: %s\n", query)
			return nil
		}

		// Sort by score
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		if maxResults > 0 && len(results) > maxResults {
			results = results[:maxResults]
		}

		fmt.Printf("Found %d result(s) for \"%s\":\n\n", len(results), query)
		for _, r := range results {
			preview := truncate(r.Content, 60)
			fmt.Printf("  [%.2f] %s  %s\n", r.Score, r.ID, r.Impetus.Label)
			fmt.Printf("              %s\n\n", preview)
		}
		return nil
	}

	results, err := c.store.Search(query, maxResults)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No beats found matching: %s\n", query)
		return nil
	}

	fmt.Printf("Found %d result(s) for \"%s\":\n\n", len(results), query)
	for _, r := range results {
		preview := truncate(r.Content, 60)
		fmt.Printf("  [%.2f] %s  %s\n", r.Score, r.ID, r.Impetus.Label)
		fmt.Printf("              %s\n\n", preview)
	}

	return nil
}

// Link adds bead IDs to a beat's linked_beads.
func (c *HumanCLI) Link(beatID string, beadIDs []string) error {
	updated, err := c.store.Update(beatID, func(b *beat.Beat) error {
		// Add new bead IDs, avoiding duplicates
		existing := make(map[string]bool)
		for _, id := range b.LinkedBeads {
			existing[id] = true
		}
		for _, id := range beadIDs {
			if !existing[id] {
				b.LinkedBeads = append(b.LinkedBeads, id)
				existing[id] = true
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to link beat: %w", err)
	}

	fmt.Printf("Updated %s\n", updated.ID)
	fmt.Printf("Linked beads: %s\n", strings.Join(updated.LinkedBeads, ", "))
	return nil
}

// Delete removes a beat by ID.
func (c *HumanCLI) Delete(id string, force bool) error {
	// First show the beat to confirm
	b, err := c.store.Get(id)
	if err != nil {
		return err
	}

	if !force {
		fmt.Printf("Deleting beat: %s\n", b.ID)
		fmt.Printf("  Impetus: %s\n", b.Impetus.Label)
		fmt.Printf("  Content: %s\n", truncate(b.Content, 60))
		fmt.Print("\nConfirm deletion? [y/N] ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Canceled.")
			return nil
		}
	}

	if err := c.store.Delete(id); err != nil {
		return fmt.Errorf("failed to delete beat: %w", err)
	}

	fmt.Printf("Deleted beat: %s\n", id)
	return nil
}

// Move exports a beat to another .beats directory and removes it from current.
func (c *HumanCLI) Move(id string, targetDir string) error {
	// Get the beat from current store
	b, err := c.store.Get(id)
	if err != nil {
		return err
	}

	// Create target store
	targetStore, err := store.NewJSONLStore(targetDir)
	if err != nil {
		return fmt.Errorf("failed to open target directory: %w", err)
	}

	// Append to target (preserves original ID and timestamps)
	if err := targetStore.Append(b); err != nil {
		return fmt.Errorf("failed to write to target: %w", err)
	}

	// Delete from source
	if err := c.store.Delete(id); err != nil {
		return fmt.Errorf("failed to delete from source: %w", err)
	}

	fmt.Printf("Moved %s to %s\n", id, targetDir)
	return nil
}

// SearchAll searches across all .beats directories under the given root.
func (c *HumanCLI) SearchAll(root string, query string, maxResults int) error {
	if maxResults <= 0 {
		maxResults = 20
	}

	projects, err := store.DiscoverBeatsProjects(root)
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Printf("No beats projects found under %s\n", root)
		return nil
	}

	type resultWithProject struct {
		Project string
		Result  beat.SearchResult
	}

	var allResults []resultWithProject

	for _, projectDir := range projects {
		projectStore, err := store.NewJSONLStore(projectDir)
		if err != nil {
			continue // Skip projects we can't open
		}

		results, err := projectStore.Search(query, 0) // Get all matches
		if err != nil {
			continue
		}

		projectName := filepath.Base(filepath.Dir(projectDir))
		for _, r := range results {
			allResults = append(allResults, resultWithProject{
				Project: projectName,
				Result:  r,
			})
		}
	}

	if len(allResults) == 0 {
		fmt.Printf("No beats found matching \"%s\" across %d projects\n", query, len(projects))
		return nil
	}

	// Sort by score descending
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Result.Score > allResults[j].Result.Score
	})

	// Limit results
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}

	fmt.Printf("Found %d result(s) for \"%s\" across %d projects:\n\n", len(allResults), query, len(projects))
	for _, r := range allResults {
		preview := truncate(r.Result.Content, 50)
		fmt.Printf("  [%.2f] [%s] %s\n", r.Result.Score, r.Project, r.Result.ID)
		fmt.Printf("         %s\n", r.Result.Impetus.Label)
		fmt.Printf("         %s\n\n", preview)
	}

	return nil
}

// ListProjects lists all beats projects under the given root.
func (c *HumanCLI) ListProjects(root string) error {
	projects, err := store.DiscoverBeatsProjects(root)
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Printf("No beats projects found under %s\n", root)
		return nil
	}

	fmt.Printf("Found %d beats project(s) under %s:\n\n", len(projects), root)
	for _, projectDir := range projects {
		info, err := store.GetProjectInfo(projectDir)
		if err != nil {
			fmt.Printf("  %s (error reading)\n", projectDir)
			continue
		}
		fmt.Printf("  %-25s %3d beats  %s\n", info.ProjectName, info.BeatCount, info.BeatsDir)
	}

	return nil
}

// GetDefaultRoot returns the default root directory for cross-project operations.
// Uses BEATS_ROOT env var if set, otherwise tries to find a reasonable default.
func GetDefaultRoot() string {
	if root := os.Getenv("BEATS_ROOT"); root != "" {
		return root
	}
	// Try common locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "werk"),
		filepath.Join(home, "work"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "code"),
		home,
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return home
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// EmbeddingsCompute generates embeddings for all beats
func (c *HumanCLI) EmbeddingsCompute() error {
	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
	}

	embStore, err := embeddings.NewStore(c.store.Dir())
	if err != nil {
		return fmt.Errorf("failed to init embedding store: %w", err)
	}

	ollama := embeddings.NewOllamaClient()
	if !ollama.IsAvailable() {
		return fmt.Errorf("ollama not available (is it running?)")
	}

	fmt.Printf("Computing embeddings for %d beats...\n", len(beats))
	result, err := embeddings.ComputeMissing(context.Background(), beats, embStore, ollama)
	if err != nil {
		return err
	}

	fmt.Printf("Done: %d computed, %d skipped, %d errors\n", result.Computed, result.Skipped, result.Errors)
	return nil
}

// EmbeddingsStatus shows embedding coverage
func (c *HumanCLI) EmbeddingsStatus() error {
	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
	}

	embStore, err := embeddings.NewStore(c.store.Dir())
	if err != nil {
		return fmt.Errorf("failed to init embedding store: %w", err)
	}

	coverage := embStore.Coverage(len(beats))
	fmt.Printf("Embeddings: %d/%d (%.1f%%)\n", embStore.Count(), len(beats), coverage)
	return nil
}

// BackfillContext updates beats without context by inferring from capture_path.
func (c *HumanCLI) BackfillContext(dryRun bool) error {
	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
	}

	fmt.Println("Backfilling beat contexts...")

	var alreadyHasContext, updatedFromPath, noContextAvailable int
	var toUpdate []string

	for _, b := range beats {
		// Skip if already has context with wald_directory
		if b.Context != nil && b.Context.WALDDirectory != "" {
			alreadyHasContext++
			continue
		}

		// Try to derive from capture_path
		capturePath := ""
		if b.Context != nil && b.Context.CapturePath != "" {
			capturePath = b.Context.CapturePath
		}

		if capturePath != "" {
			waldDir, confidence := resolveWALDDirectory(capturePath)
			if waldDir != "" && confidence > 0 {
				toUpdate = append(toUpdate, b.ID)
				updatedFromPath++
				continue
			}
		}

		noContextAvailable++
	}

	fmt.Printf("Analyzed %d beats\n", len(beats))
	fmt.Printf("  - %d already have context\n", alreadyHasContext)
	fmt.Printf("  - %d updated from capture_path\n", updatedFromPath)
	fmt.Printf("  - %d no context available\n", noContextAvailable)

	if dryRun {
		fmt.Printf("\n[dry-run] Would update %d beats.\n", len(toUpdate))
		return nil
	}

	// Actually update the beats
	for _, id := range toUpdate {
		_, err := c.store.Update(id, func(b *beat.Beat) error {
			capturePath := ""
			if b.Context != nil {
				capturePath = b.Context.CapturePath
			}

			waldDir, confidence := resolveWALDDirectory(capturePath)
			b.Context = &beat.Context{
				CapturePath:     capturePath,
				WALDDirectory:   waldDir,
				InferenceMethod: "backfill",
				Confidence:      confidence * 0.8, // Mark as backfill with reduced confidence
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to update beat %s: %w", id, err)
		}
	}

	fmt.Printf("\nUpdated %d beats.\n", len(toUpdate))
	return nil
}

// WALDConfig represents the WALD.yaml structure.
type WALDConfig struct {
	Directories []WALDDirectory `yaml:"directories"`
}

// WALDDirectory represents a directory entry in WALD.yaml.
type WALDDirectory struct {
	Path    string      `yaml:"path"`
	Purpose string      `yaml:"purpose"`
	State   string      `yaml:"state"`
	Gravity string      `yaml:"gravity"`
	Claims  *WALDClaims `yaml:"claims,omitempty"`
}

// WALDClaims represents the claims a directory makes on beats.
type WALDClaims struct {
	Clusters    []string `yaml:"clusters,omitempty"`
	Topics      []string `yaml:"topics,omitempty"`
	Keywords    []string `yaml:"keywords,omitempty"`
	Cooperators []string `yaml:"cooperators,omitempty"`
}

// loadWALDConfig loads WALD.yaml from the werk root.
func loadWALDConfig(werkRoot string) (*WALDConfig, error) {
	waldPath := filepath.Join(werkRoot, "WALD.yaml")
	data, err := os.ReadFile(waldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WALD.yaml: %w", err)
	}
	var config WALDConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse WALD.yaml: %w", err)
	}
	return &config, nil
}

// claimedBeat represents a beat matched via a claim.
type claimedBeat struct {
	Beat       beat.Beat
	MatchType  string // "cluster", "topic", "keyword", "cooperator"
	MatchValue string // the specific claim that matched
}

// ContextRobotOutput is JSON output for --robot context.
type ContextRobotOutput struct {
	Directory          string               `json:"directory"`
	Temperature        float64              `json:"temperature"`
	Claims             *ContextClaims       `json:"claims"`
	Beats              *ContextBeatsByClaim `json:"beats"`
	TotalRelevantBeats int                  `json:"total_relevant_beats"`
	Shown              int                  `json:"shown"`
}

// ContextClaims represents the claims for JSON output.
type ContextClaims struct {
	Clusters    []string `json:"clusters,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Cooperators []string `json:"cooperators,omitempty"`
}

// ContextBeatsByClaim groups beats by claim type for JSON output.
type ContextBeatsByClaim struct {
	ByCluster    []ClaimedBeatOutput `json:"by_cluster,omitempty"`
	ByTopic      []ClaimedBeatOutput `json:"by_topic,omitempty"`
	ByKeyword    []ClaimedBeatOutput `json:"by_keyword,omitempty"`
	ByCooperator []ClaimedBeatOutput `json:"by_cooperator,omitempty"`
}

// ClaimedBeatOutput represents a beat in context output.
type ClaimedBeatOutput struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	AgeDays     int       `json:"age_days"`
	Preview     string    `json:"preview"`
	MatchValue  string    `json:"match_value"`
	FullContent string    `json:"full_content,omitempty"`
}

// Context surfaces relevant beats for a given WALD directory path via claims.
func (c *HumanCLI) Context(path string, limit int) error {
	return c.ContextWithOptions(path, limit, false)
}

// ContextWithOptions surfaces relevant beats with optional JSON output.
func (c *HumanCLI) ContextWithOptions(path string, limit int, robotOutput bool) error {
	// Default to current working directory
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Make path absolute if relative
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		path = filepath.Join(cwd, path)
	}

	// Find werk root and convert to WALD relative path
	waldPath, werkRoot := resolveToWALDPath(path)
	if werkRoot == "" {
		return fmt.Errorf("not in a WALD workspace (no WALD.yaml found)")
	}

	// Handle root directory case
	if waldPath == "" || waldPath == "." {
		waldPath = "(werk root)"
	}

	// Load WALD.yaml to get claims
	waldConfig, err := loadWALDConfig(werkRoot)
	if err != nil {
		return fmt.Errorf("failed to load WALD.yaml: %w", err)
	}

	// Find the directory entry and its claims
	var dirEntry *WALDDirectory
	for i := range waldConfig.Directories {
		if waldConfig.Directories[i].Path == waldPath {
			dirEntry = &waldConfig.Directories[i]
			break
		}
	}

	// Get claims (may be nil/empty)
	var claims *WALDClaims
	if dirEntry != nil && dirEntry.Claims != nil {
		claims = dirEntry.Claims
	}

	// Load all beats
	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
	}

	// Find beats by claim type
	seenIDs := make(map[string]bool)
	var clusterBeats, topicBeats, keywordBeats, cooperatorBeats []claimedBeat

	if claims != nil {
		// Topic matches: beat content contains claimed topic (case-insensitive)
		for _, topic := range claims.Topics {
			topicLower := strings.ToLower(topic)
			for _, b := range beats {
				if seenIDs[b.ID] {
					continue
				}
				if strings.Contains(strings.ToLower(b.Content), topicLower) {
					topicBeats = append(topicBeats, claimedBeat{Beat: b, MatchType: "topic", MatchValue: topic})
					seenIDs[b.ID] = true
				}
			}
		}

		// Keyword matches: beat content contains claimed keyword (case-insensitive)
		for _, keyword := range claims.Keywords {
			keywordLower := strings.ToLower(keyword)
			for _, b := range beats {
				if seenIDs[b.ID] {
					continue
				}
				if strings.Contains(strings.ToLower(b.Content), keywordLower) {
					keywordBeats = append(keywordBeats, claimedBeat{Beat: b, MatchType: "keyword", MatchValue: keyword})
					seenIDs[b.ID] = true
				}
			}
		}

		// Cooperator matches: beat content mentions claimed cooperator
		for _, coop := range claims.Cooperators {
			coopLower := strings.ToLower(coop)
			for _, b := range beats {
				if seenIDs[b.ID] {
					continue
				}
				contentLower := strings.ToLower(b.Content)
				// Check for cooperator name directly or as cooperators/name path
				if strings.Contains(contentLower, coopLower) || strings.Contains(contentLower, "cooperators/"+coopLower) {
					cooperatorBeats = append(cooperatorBeats, claimedBeat{Beat: b, MatchType: "cooperator", MatchValue: coop})
					seenIDs[b.ID] = true
				}
			}
		}

		// Cluster matches would require loading btv-cache.json - skip for now if empty
		// (Clusters are more complex; topic/keyword/cooperator cover most use cases)
	}

	// Sort each category by recency
	sortClaimedBeats := func(cb []claimedBeat) {
		sort.Slice(cb, func(i, j int) bool {
			return cb[i].Beat.CreatedAt.After(cb[j].Beat.CreatedAt)
		})
	}
	sortClaimedBeats(clusterBeats)
	sortClaimedBeats(topicBeats)
	sortClaimedBeats(keywordBeats)
	sortClaimedBeats(cooperatorBeats)

	totalBeats := len(clusterBeats) + len(topicBeats) + len(keywordBeats) + len(cooperatorBeats)

	// Calculate temperature based on activity (simplified)
	temperature := 0.0
	if totalBeats > 0 {
		// Simple heuristic: more beats = hotter, recent beats = hotter
		recentCount := 0
		now := time.Now()
		for _, cb := range append(append(append(clusterBeats, topicBeats...), keywordBeats...), cooperatorBeats...) {
			if now.Sub(cb.Beat.CreatedAt).Hours() < 24*7 { // within a week
				recentCount++
			}
		}
		temperature = float64(recentCount) / float64(totalBeats)
		if temperature > 1.0 {
			temperature = 1.0
		}
	}

	if robotOutput {
		return c.outputContextJSON(waldPath, temperature, claims, clusterBeats, topicBeats, keywordBeats, cooperatorBeats, limit)
	}

	return c.outputContextHuman(waldPath, temperature, claims, clusterBeats, topicBeats, keywordBeats, cooperatorBeats, totalBeats, limit)
}

func (c *HumanCLI) outputContextJSON(waldPath string, temperature float64, claims *WALDClaims,
	clusterBeats, topicBeats, keywordBeats, cooperatorBeats []claimedBeat, limit int) error {

	toOutput := func(cbs []claimedBeat, maxItems int) []ClaimedBeatOutput {
		result := make([]ClaimedBeatOutput, 0)
		for i, cb := range cbs {
			if i >= maxItems {
				break
			}
			result = append(result, ClaimedBeatOutput{
				ID:          cb.Beat.ID,
				CreatedAt:   cb.Beat.CreatedAt,
				AgeDays:     int(time.Since(cb.Beat.CreatedAt).Hours() / 24),
				Preview:     truncate(cb.Beat.Content, 80),
				MatchValue:  cb.MatchValue,
				FullContent: cb.Beat.Content,
			})
		}
		return result
	}

	var claimsOut *ContextClaims
	if claims != nil {
		claimsOut = &ContextClaims{
			Clusters:    claims.Clusters,
			Topics:      claims.Topics,
			Keywords:    claims.Keywords,
			Cooperators: claims.Cooperators,
		}
	}

	totalBeats := len(clusterBeats) + len(topicBeats) + len(keywordBeats) + len(cooperatorBeats)
	shown := 0
	byCluster := toOutput(clusterBeats, limit)
	byTopic := toOutput(topicBeats, limit)
	byKeyword := toOutput(keywordBeats, limit)
	byCooperator := toOutput(cooperatorBeats, limit)
	shown = len(byCluster) + len(byTopic) + len(byKeyword) + len(byCooperator)

	output := ContextRobotOutput{
		Directory:   waldPath,
		Temperature: temperature,
		Claims:      claimsOut,
		Beats: &ContextBeatsByClaim{
			ByCluster:    byCluster,
			ByTopic:      byTopic,
			ByKeyword:    byKeyword,
			ByCooperator: byCooperator,
		},
		TotalRelevantBeats: totalBeats,
		Shown:              shown,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func (c *HumanCLI) outputContextHuman(waldPath string, temperature float64, claims *WALDClaims,
	clusterBeats, topicBeats, keywordBeats, cooperatorBeats []claimedBeat, totalBeats, limit int) error {

	// Header
	tempLabel := "cold"
	tempArrow := ""
	if temperature >= 0.7 {
		tempLabel = "hot"
		tempArrow = " ↑"
	} else if temperature >= 0.4 {
		tempLabel = "warm"
	} else if temperature >= 0.15 {
		tempLabel = "cool"
	}

	fmt.Printf("Context: %s\n", waldPath)
	fmt.Printf("Temperature: %.2f (%s)%s\n", temperature, tempLabel, tempArrow)

	// Claims summary
	if claims != nil {
		parts := []string{}
		if len(claims.Clusters) > 0 {
			parts = append(parts, fmt.Sprintf("%d clusters", len(claims.Clusters)))
		}
		if len(claims.Topics) > 0 {
			parts = append(parts, fmt.Sprintf("%d topics", len(claims.Topics)))
		}
		if len(claims.Keywords) > 0 {
			parts = append(parts, fmt.Sprintf("%d keywords", len(claims.Keywords)))
		}
		if len(claims.Cooperators) > 0 {
			parts = append(parts, fmt.Sprintf("%d cooperators", len(claims.Cooperators)))
		}
		if len(parts) > 0 {
			fmt.Printf("Claims: %s\n", strings.Join(parts, ", "))
		}
	}
	fmt.Println()

	shownTotal := 0

	// Output each section
	printSection := func(title string, cbs []claimedBeat) {
		if len(cbs) == 0 {
			return
		}
		fmt.Printf("─── %s ─────────────────────────────────────────────\n", title)
		shown := limit
		if shown > len(cbs) {
			shown = len(cbs)
		}
		for _, cb := range cbs[:shown] {
			age := formatAge(cb.Beat.CreatedAt)
			preview := truncate(cb.Beat.Content, 50)
			fmt.Printf("  [%s] %s  %s\n", age, cb.Beat.ID, preview)
			fmt.Printf("       matches %s: %s\n", cb.MatchType, cb.MatchValue)
		}
		fmt.Println()
		shownTotal += shown
	}

	printSection("From claimed clusters", clusterBeats)
	printSection("From topic matches", topicBeats)
	printSection("From keyword matches", keywordBeats)
	printSection("From cooperator mentions", cooperatorBeats)

	if totalBeats == 0 {
		fmt.Println("  (no beats found matching claims)")
		fmt.Println()
	}

	if totalBeats > 0 {
		fmt.Printf("[%d of %d beats shown]\n", shownTotal, totalBeats)
	}
	fmt.Println()
	fmt.Println("Note: Beats are not filed here. Relevance is via claims.")

	return nil
}

// resolveToWALDPath resolves an absolute path to a WALD-relative path.
// Returns (waldPath, werkRoot) where werkRoot is empty if not in a WALD workspace.
func resolveToWALDPath(absPath string) (string, string) {
	dir := absPath
	for {
		waldFile := filepath.Join(dir, "WALD.yaml")
		if _, err := os.Stat(waldFile); err == nil {
			relPath, _ := filepath.Rel(dir, absPath)
			if relPath == "" {
				relPath = "."
			}
			return relPath, dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// formatAge formats a time as a relative age string (e.g., "3d", "2h").
func formatAge(t time.Time) string {
	dur := time.Since(t)
	days := int(dur.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(dur.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	minutes := int(dur.Minutes())
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "now"
}

// SemanticSearch performs semantic search using embeddings
func (c *HumanCLI) SemanticSearch(query string, maxResults int) error {
	if maxResults <= 0 {
		maxResults = 20
	}

	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
	}

	embStore, err := embeddings.NewStore(c.store.Dir())
	if err != nil {
		return fmt.Errorf("failed to init embedding store: %w", err)
	}

	ollama := embeddings.NewOllamaClient()
	if !ollama.IsAvailable() {
		return fmt.Errorf("ollama not available (is it running?)")
	}

	results, err := embeddings.SemanticSearch(context.Background(), query, beats, embStore, ollama, maxResults)
	if err != nil {
		return fmt.Errorf("semantic search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No beats found for: %s\n", query)
		return nil
	}

	fmt.Printf("Found %d result(s) for \"%s\" (semantic):\n\n", len(results), query)
	for _, r := range results {
		preview := truncate(r.Content, 60)
		fmt.Printf("  [%.3f] %s  %s\n", r.Score, r.ID, r.Impetus.Label)
		fmt.Printf("              %s\n\n", preview)
	}
	return nil
}
