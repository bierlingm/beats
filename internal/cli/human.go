package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
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

// Add creates a new beat with the given content.
func (c *HumanCLI) Add(content string, impetusLabel string) error {
	seq, err := c.store.NextSequence()
	if err != nil {
		return fmt.Errorf("failed to get sequence: %w", err)
	}

	impetus := beat.Impetus{
		Label: impetusLabel,
	}
	if impetusLabel == "" {
		impetus.Label = "Manual entry"
	}

	b := &beat.Beat{
		ID:          beat.GenerateIDWithSequence(time.Now().UTC(), seq),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Impetus:     impetus,
		Content:     content,
		References:  []beat.Reference{},
		Entities:    []beat.Entity{},
		LinkedBeads: []string{},
	}

	if err := c.store.Append(b); err != nil {
		return fmt.Errorf("failed to save beat: %w", err)
	}

	fmt.Printf("Created beat: %s\n", b.ID)
	return nil
}

// List displays all beats.
func (c *HumanCLI) List() error {
	beats, err := c.store.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read beats: %w", err)
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

// Search finds beats matching the query.
func (c *HumanCLI) Search(query string, maxResults int) error {
	if maxResults <= 0 {
		maxResults = 20
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

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
