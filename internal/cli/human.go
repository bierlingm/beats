package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
	"github.com/bierlingm/beats/internal/capture"
	"github.com/bierlingm/beats/internal/embeddings"
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

	b := &beat.Beat{
		ID:          beat.GenerateIDWithSequence(time.Now().UTC(), seq),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Impetus:     imp,
		Content:     finalContent,
		References:  []beat.Reference{},
		Entities:    []beat.Entity{},
		LinkedBeads: []string{},
	}

	if sessionID := os.Getenv("FACTORY_SESSION_ID"); sessionID != "" {
		b.SessionID = sessionID
	}

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
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled.")
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
