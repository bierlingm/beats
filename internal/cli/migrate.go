package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
	"github.com/bierlingm/beats/internal/store"
)

// MigrateOptions contains options for the migrate command.
type MigrateOptions struct {
	DryRun  bool
	Cleanup bool
	Force   bool
}

// MigrateConsolidate merges all scattered .beats/ directories into the global store.
func (c *HumanCLI) MigrateConsolidate(opts MigrateOptions) error {
	werkRoot := "/Users/moritzbierling/werk"
	globalStore := store.GlobalBeatsStore

	// Find all .beats directories
	var scatteredStores []string
	err := filepath.WalkDir(werkRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		// Skip hidden dirs except .beats
		if strings.HasPrefix(name, ".") && name != ".beats" {
			return filepath.SkipDir
		}

		// Skip common non-project dirs
		skipDirs := map[string]bool{
			"node_modules": true,
			"vendor":       true,
			"__pycache__":  true,
			".git":         true,
		}
		if skipDirs[name] {
			return filepath.SkipDir
		}

		// Check for .beats with beats.jsonl
		if name == ".beats" {
			beatsFile := filepath.Join(path, "beats.jsonl")
			if _, err := os.Stat(beatsFile); err == nil {
				// Skip the global store itself
				if path != globalStore {
					scatteredStores = append(scatteredStores, path)
				}
			}
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan for .beats directories: %w", err)
	}

	if len(scatteredStores) == 0 {
		fmt.Println("No scattered .beats directories found to migrate.")
		return nil
	}

	fmt.Printf("Found %d scattered .beats directories:\n", len(scatteredStores))
	for _, s := range scatteredStores {
		fmt.Printf("  - %s\n", s)
	}
	fmt.Println()

	// Load existing beats from global store (for deduplication)
	existingBeats := make(map[string]beat.Beat)
	globalBeatsFile := filepath.Join(globalStore, "beats.jsonl")
	if f, err := os.Open(globalBeatsFile); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			var b beat.Beat
			if err := json.Unmarshal([]byte(line), &b); err == nil {
				existingBeats[b.ID] = b
			}
		}
		f.Close()
	}

	var totalMigrated, totalDuplicates int

	// Process each scattered store
	for _, storePath := range scatteredStores {
		beatsFile := filepath.Join(storePath, "beats.jsonl")

		// Derive wald_directory from path
		relPath, err := filepath.Rel(werkRoot, filepath.Dir(storePath))
		if err != nil {
			relPath = filepath.Dir(storePath)
		}

		// Read beats from this store
		f, err := os.Open(beatsFile)
		if err != nil {
			fmt.Printf("  Warning: could not read %s: %v\n", beatsFile, err)
			continue
		}

		var beatsToMigrate []beat.Beat
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			var b beat.Beat
			if err := json.Unmarshal([]byte(line), &b); err != nil {
				fmt.Printf("  Warning: could not parse beat in %s: %v\n", beatsFile, err)
				continue
			}

			// Check for duplicate
			if existing, ok := existingBeats[b.ID]; ok {
				// Keep the more recent one
				if b.UpdatedAt.After(existing.UpdatedAt) {
					beatsToMigrate = append(beatsToMigrate, b)
				}
				totalDuplicates++
				continue
			}

			beatsToMigrate = append(beatsToMigrate, b)
		}
		f.Close()

		if len(beatsToMigrate) == 0 {
			fmt.Printf("  %s: 0 beats to migrate\n", relPath)
			continue
		}

		fmt.Printf("  %s: %d beats to migrate\n", relPath, len(beatsToMigrate))

		if opts.DryRun {
			totalMigrated += len(beatsToMigrate)
			continue
		}

		// Add _legacy_context to each beat and append to global store
		globalFile, err := os.OpenFile(globalBeatsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open global store: %w", err)
		}

		for _, b := range beatsToMigrate {
			// Add legacy context
			legacyContext := map[string]interface{}{
				"wald_directory": relPath,
				"migrated_from":  storePath,
				"migrated_at":    time.Now().UTC().Format(time.RFC3339),
			}

			// Marshal beat to map to add _legacy_context
			beatData, _ := json.Marshal(b)
			var beatMap map[string]interface{}
			_ = json.Unmarshal(beatData, &beatMap)
			beatMap["_legacy_context"] = legacyContext

			// Write to global store
			data, _ := json.Marshal(beatMap)
			_, _ = globalFile.Write(append(data, '\n'))

			existingBeats[b.ID] = b
			totalMigrated++
		}
		globalFile.Close()

		// Create backup of original
		backupPath := beatsFile + ".bak"
		if err := os.Rename(beatsFile, backupPath); err != nil {
			fmt.Printf("  Warning: could not backup %s: %v\n", beatsFile, err)
		}
	}

	fmt.Println()
	if opts.DryRun {
		fmt.Printf("[dry-run] Would migrate %d beats from %d stores, %d duplicates resolved\n",
			totalMigrated, len(scatteredStores), totalDuplicates)
	} else {
		fmt.Printf("Migrated %d beats from %d stores, %d duplicates resolved\n",
			totalMigrated, len(scatteredStores), totalDuplicates)
		fmt.Println("Original .beats/beats.jsonl files renamed to .bak")
	}

	return nil
}

// MigrateCleanup removes old .beats/ directories after verifying migration
func (c *HumanCLI) MigrateCleanup(opts MigrateOptions) error {
	werkRoot := "/Users/moritzbierling/werk"
	globalStore := store.GlobalBeatsStore
	globalBeatsFile := filepath.Join(globalStore, "beats.jsonl")

	// Verify global store exists and has beats
	if _, err := os.Stat(globalBeatsFile); err != nil {
		return fmt.Errorf("global store not found at %s - run 'bt migrate --consolidate' first", globalStore)
	}

	// Load global store beats
	globalBeats := make(map[string]bool)
	f, err := os.Open(globalBeatsFile)
	if err != nil {
		return fmt.Errorf("failed to open global store: %w", err)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var b beat.Beat
		if err := json.Unmarshal([]byte(line), &b); err == nil {
			globalBeats[b.ID] = true
		}
	}
	f.Close()

	if len(globalBeats) == 0 {
		return fmt.Errorf("global store is empty - run 'bt migrate --consolidate' first")
	}

	// Find all old .beats directories (excluding global store)
	var oldStores []string
	var oldStoreBeats = make(map[string][]string) // path -> beat IDs

	err = filepath.WalkDir(werkRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		// Skip hidden dirs except .beats
		if strings.HasPrefix(name, ".") && name != ".beats" {
			return filepath.SkipDir
		}

		// Skip common non-project dirs
		skipDirs := map[string]bool{
			"node_modules": true,
			"vendor":       true,
			"__pycache__":  true,
			".git":         true,
		}
		if skipDirs[name] {
			return filepath.SkipDir
		}

		// Check for .beats with beats.jsonl or .bak
		if name == ".beats" {
			// Skip the global store itself
			if path == globalStore {
				return filepath.SkipDir
			}

			// Check for beats.jsonl or beats.jsonl.bak
			beatsFile := filepath.Join(path, "beats.jsonl")
			bakFile := filepath.Join(path, "beats.jsonl.bak")

			hasBeats := false
			var beatsToCheck string
			if _, err := os.Stat(beatsFile); err == nil {
				hasBeats = true
				beatsToCheck = beatsFile
			} else if _, err := os.Stat(bakFile); err == nil {
				hasBeats = true
				beatsToCheck = bakFile
			}

			if hasBeats {
				oldStores = append(oldStores, path)

				// Read beat IDs from this store
				bf, err := os.Open(beatsToCheck)
				if err == nil {
					scanner := bufio.NewScanner(bf)
					for scanner.Scan() {
						line := scanner.Text()
						if strings.TrimSpace(line) == "" {
							continue
						}
						var b beat.Beat
						if err := json.Unmarshal([]byte(line), &b); err == nil {
							oldStoreBeats[path] = append(oldStoreBeats[path], b.ID)
						}
					}
					bf.Close()
				}
			}
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan for .beats directories: %w", err)
	}

	if len(oldStores) == 0 {
		fmt.Println("No old .beats directories found to clean up.")
		return nil
	}

	fmt.Println("Migration cleanup verification:")
	fmt.Println()
	fmt.Printf("Global store: %s (%d beats)\n", globalBeatsFile, len(globalBeats))
	fmt.Println()
	fmt.Println("Old stores to remove:")

	allMigrated := true
	for _, storePath := range oldStores {
		beatIDs := oldStoreBeats[storePath]
		migratedCount := 0
		for _, id := range beatIDs {
			if globalBeats[id] {
				migratedCount++
			}
		}

		status := "✓ all migrated"
		if migratedCount < len(beatIDs) {
			status = fmt.Sprintf("✗ %d of %d migrated", migratedCount, len(beatIDs))
			allMigrated = false
		}

		relPath, _ := filepath.Rel(werkRoot, storePath)
		fmt.Printf("  %s (%d beats) %s\n", relPath, len(beatIDs), status)
	}

	if !allMigrated {
		fmt.Println()
		fmt.Println("Warning: Some beats were not migrated. Run 'bt migrate --consolidate' first.")
		if !opts.Force {
			return fmt.Errorf("cleanup aborted - not all beats migrated")
		}
		fmt.Println("Proceeding with --force...")
	}

	fmt.Println()

	if !opts.Force {
		fmt.Println("Run with --force to delete old stores.")
		fmt.Printf("Backups were created at %s/migration-backup/ during consolidation\n", globalStore)
		return nil
	}

	// Create archive directory
	archiveDir := filepath.Join(globalStore, "archived-stores")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Move old stores to archive
	var removed, failed int
	for _, storePath := range oldStores {
		relPath, _ := filepath.Rel(werkRoot, storePath)
		archivePath := filepath.Join(archiveDir, strings.ReplaceAll(relPath, "/", "_"))

		if opts.DryRun {
			fmt.Printf("[dry-run] Would move %s to %s\n", relPath, archivePath)
			removed++
			continue
		}

		// Move to archive
		if err := os.Rename(storePath, archivePath); err != nil {
			fmt.Printf("Failed to archive %s: %v\n", relPath, err)
			failed++
			continue
		}

		fmt.Printf("Archived %s\n", relPath)
		removed++
	}

	fmt.Println()
	if opts.DryRun {
		fmt.Printf("[dry-run] Would archive %d old .beats directories\n", removed)
	} else {
		fmt.Printf("Archived %d old .beats directories to %s\n", removed, archiveDir)
		if failed > 0 {
			fmt.Printf("%d directories could not be archived\n", failed)
		}
	}

	return nil
}
