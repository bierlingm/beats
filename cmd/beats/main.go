package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/cli"
	"github.com/bierlingm/beats/internal/hooks"
	"github.com/bierlingm/beats/internal/store"
)

const version = "0.5.0"

// multiFlag allows a flag to be specified multiple times
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]

	// Check for robot commands first (they're flags, not subcommands)
	if len(args) > 0 && strings.HasPrefix(args[0], "--robot-") {
		return handleRobotCommand(args[0], args[1:])
	}

	// Check for global flags
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-version":
			fmt.Printf("beats v%s\n", version)
			return nil
		case "--help", "-help", "-h":
			printUsage()
			return nil
		}
	}

	// No args = show help
	if len(args) == 0 {
		printUsage()
		return nil
	}

	// Handle subcommands
	cmd := args[0]
	cmdArgs := args[1:]

	// Handle prime command separately (no store needed initially)
	if cmd == "prime" {
		beatsDir := ""
		for i, arg := range cmdArgs {
			if arg == "--dir" && i+1 < len(cmdArgs) {
				beatsDir = cmdArgs[i+1]
				break
			}
		}
		return handlePrimeCommand(beatsDir)
	}

	return handleHumanCommand(cmd, cmdArgs)
}

func handleRobotCommand(cmd string, args []string) error {
	// Parse optional --dir flag for robot commands
	robotFlags := flag.NewFlagSet("robot", flag.ExitOnError)
	beatsDir := robotFlags.String("dir", "", "Beats directory")
	if err := robotFlags.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	jsonStore, err := store.NewJSONLStore(*beatsDir)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	cli.SetJSONOutput(os.Stdout)
	robotCLI := cli.NewRobotCLI(jsonStore)

	switch cmd {
	case "--robot-help":
		return robotCLI.Help()
	case "--robot-propose-beat":
		return robotCLI.ProposeBeat(os.Stdin)
	case "--robot-commit-beat":
		return robotCLI.CommitBeat(os.Stdin)
	case "--robot-search":
		return robotCLI.Search(os.Stdin)
	case "--robot-brief":
		return robotCLI.Brief(os.Stdin)
	case "--robot-context-for-bead":
		return robotCLI.ContextForBead(os.Stdin)
	case "--robot-map-beats-to-beads":
		return robotCLI.MapBeatsToBeads(os.Stdin)
	case "--robot-diff":
		return robotCLI.Diff(os.Stdin)
	case "--robot-link-beat":
		return robotCLI.LinkBeat(os.Stdin)
	case "--robot-synthesis-status":
		return robotCLI.SynthesisStatus()
	case "--robot-synthesis-clear":
		return robotCLI.SynthesisClear()
	case "--robot-context":
		return robotCLI.Context(os.Stdin)
	case "--robot-edit":
		return robotCLI.Edit(os.Stdin)
	case "--robot-amend":
		return robotCLI.Amend(os.Stdin)
	case "--robot-import":
		return robotCLI.Import(os.Stdin)
	case "--robot-export":
		return robotCLI.Export(os.Stdin)
	case "--robot-redate":
		return robotCLI.Redate(os.Stdin)
	default:
		return fmt.Errorf("unknown robot command: %s", cmd)
	}
}

func handleExportCommand(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	beatsDir := fs.String("dir", "", "Beats directory")
	exportFormat := fs.String("format", "jsonl", "Output format: json, jsonl, csv")
	exportSince := fs.String("since", "", "Filter by created_at >= datetime")
	exportUntil := fs.String("until", "", "Filter by created_at <= datetime")
	exportImpetus := fs.String("impetus", "", "Filter by impetus label (substring match)")
	exportQuery := fs.String("query", "", "Filter by content (substring match)")
	exportOutput := fs.String("output", "", "Output file (default: stdout)")
	exportOutputShort := fs.String("o", "", "Output file (short)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	jsonStore, err := store.NewJSONLStore(*beatsDir)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	output := *exportOutput
	if output == "" {
		output = *exportOutputShort
	}

	humanCLI := cli.NewHumanCLI(jsonStore)
	return humanCLI.Export(cli.ExportOptions{
		Format:  *exportFormat,
		Since:   *exportSince,
		Until:   *exportUntil,
		Impetus: *exportImpetus,
		Query:   *exportQuery,
		Output:  output,
	})
}

func handleHumanCommand(cmd string, args []string) error {
	// Handle export command separately with its own flag set
	if cmd == "export" {
		return handleExportCommand(args)
	}

	// Create flag set for subcommand
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	beatsDir := fs.String("dir", "", "Beats directory")
	impetusLabel := fs.String("impetus", "", "Impetus label for 'add' command")
	maxResults := fs.Int("max", 20, "Maximum results for 'search' command")
	force := fs.Bool("force", false, "Skip confirmation for delete")
	targetDir := fs.String("to", "", "Target directory for move command")
	searchAll := fs.Bool("all", false, "Search across all projects")
	rootDir := fs.String("root", "", "Root directory for cross-project operations")
	sessionFilter := fs.String("session", "", "Filter by session ID (use 'current' for FACTORY_SESSION_ID)")
	dryRun := fs.Bool("dry-run", false, "Show what would be done without making changes")
	limit := fs.Int("limit", 10, "Maximum results per category for context command")

	// Quick capture flags
	webURL := fs.String("web", "", "Capture from web URL")
	webURLShort := fs.String("w", "", "Capture from web URL (short)")
	githubRef := fs.String("github", "", "GitHub reference (owner/repo)")
	githubRefShort := fs.String("g", "", "GitHub reference (short)")
	twitterURL := fs.String("twitter", "", "X/Twitter URL")
	twitterURLShort := fs.String("x", "", "X/Twitter URL (short)")
	coaching := fs.Bool("coaching", false, "Mark as coaching insight")
	coachingShort := fs.Bool("c", false, "Mark as coaching (short)")
	sessionInsight := fs.Bool("session-insight", false, "Mark as session insight")
	sessionInsightShort := fs.Bool("s", false, "Mark as session insight (short)")
	dateStr := fs.String("date", "", "Backdate beat (ISO8601 or relative: yesterday, 3d ago)")
	dateStrShort := fs.String("d", "", "Backdate beat (short)")
	searchSemantic := fs.Bool("semantic", false, "Use semantic search")
	robotOutput := fs.Bool("robot", false, "Output JSON (for context command)")
	consolidate := fs.Bool("consolidate", false, "Consolidate scattered .beats/ into global store")
	cleanup := fs.Bool("cleanup", false, "Remove old .beats/ directories after migration verification")

	// Edit command flags
	editContent := fs.String("content", "", "New content for beat (edit command)")
	addRef := multiFlag{}
	fs.Var(&addRef, "add-ref", "Add reference (kind:locator)")
	rmRef := multiFlag{}
	fs.Var(&rmRef, "rm-ref", "Remove reference by locator")
	addBead := multiFlag{}
	fs.Var(&addBead, "add-bead", "Link a bead")
	rmBead := multiFlag{}
	fs.Var(&rmBead, "rm-bead", "Unlink a bead")

	if err := fs.Parse(args); err != nil {
		return err
	}

	jsonStore, err := store.NewJSONLStore(*beatsDir)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	humanCLI := cli.NewHumanCLI(jsonStore)
	cmdArgs := fs.Args()

	switch cmd {
	case "add":
		// Resolve short flags
		web := *webURL
		if web == "" {
			web = *webURLShort
		}
		github := *githubRef
		if github == "" {
			github = *githubRefShort
		}
		twitter := *twitterURL
		if twitter == "" {
			twitter = *twitterURLShort
		}
		isCoaching := *coaching || *coachingShort
		isSession := *sessionInsight || *sessionInsightShort

		// Resolve date flag
		dateFlagVal := *dateStr
		if dateFlagVal == "" {
			dateFlagVal = *dateStrShort
		}
		var parsedDate *time.Time
		if dateFlagVal != "" {
			t, err := cli.ParseRelativeDate(dateFlagVal)
			if err != nil {
				return fmt.Errorf("invalid date: %w", err)
			}
			parsedDate = &t
		}

		// Content is optional when using capture flags
		content := strings.Join(cmdArgs, " ")
		if web == "" && github == "" && twitter == "" && content == "" {
			return fmt.Errorf("add requires content argument or capture flag (-w, -g, -x)")
		}

		return humanCLI.AddWithOptions(cli.AddOptions{
			Content:      content,
			ImpetusLabel: *impetusLabel,
			WebURL:       web,
			GitHubRef:    github,
			TwitterURL:   twitter,
			Coaching:     isCoaching,
			Session:      isSession,
			Date:         parsedDate,
		})

	case "list":
		return humanCLI.List(*sessionFilter)

	case "show":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("show requires beat ID argument")
		}
		return humanCLI.Show(cmdArgs[0])

	case "search":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("search requires query argument")
		}
		query := strings.Join(cmdArgs, " ")
		if *searchSemantic {
			return humanCLI.SemanticSearch(query, *maxResults)
		}
		if *searchAll {
			root := *rootDir
			if root == "" {
				root = cli.GetDefaultRoot()
			}
			return humanCLI.SearchAll(root, query, *maxResults)
		}
		return humanCLI.Search(query, *maxResults, *sessionFilter)

	case "projects":
		root := *rootDir
		if root == "" {
			root = cli.GetDefaultRoot()
		}
		return humanCLI.ListProjects(root)

	case "link":
		if len(cmdArgs) < 2 {
			return fmt.Errorf("link requires beat ID and at least one bead ID")
		}
		beatID := cmdArgs[0]
		beadIDs := cmdArgs[1:]
		return humanCLI.Link(beatID, beadIDs)

	case "delete", "rm":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("delete requires beat ID argument")
		}
		return humanCLI.Delete(cmdArgs[0], *force)

	case "move", "mv":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("move requires beat ID argument")
		}
		if *targetDir == "" {
			return fmt.Errorf("move requires --to <directory> flag")
		}
		return humanCLI.Move(cmdArgs[0], *targetDir)

	case "hooks":
		return handleHooksCommand(jsonStore.Dir(), cmdArgs)

	case "where":
		// Show which .beats directory is being used
		fmt.Printf("Beats directory: %s\n", jsonStore.Dir())
		fmt.Printf("Beats file: %s\n", jsonStore.Path())
		return nil

	case "embeddings":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("embeddings requires subcommand: compute, status")
		}
		switch cmdArgs[0] {
		case "compute":
			return humanCLI.EmbeddingsCompute()
		case "status":
			return humanCLI.EmbeddingsStatus()
		default:
			return fmt.Errorf("unknown embeddings subcommand: %s", cmdArgs[0])
		}

	case "backfill-context":
		return humanCLI.BackfillContext(*dryRun)

	case "migrate":
		if !*consolidate && !*cleanup {
			return fmt.Errorf("migrate requires --consolidate or --cleanup flag")
		}
		if *cleanup {
			return humanCLI.MigrateCleanup(cli.MigrateOptions{DryRun: *dryRun, Force: *force, Cleanup: true})
		}
		return humanCLI.MigrateConsolidate(cli.MigrateOptions{DryRun: *dryRun})

	case "context":
		path := ""
		if len(cmdArgs) > 0 {
			path = cmdArgs[0]
		}
		return humanCLI.ContextWithOptions(path, *limit, *robotOutput)

	case "edit":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("edit requires beat ID argument")
		}
		dateFlagVal := *dateStr
		if dateFlagVal == "" {
			dateFlagVal = *dateStrShort
		}
		return humanCLI.Edit(cmdArgs[0], cli.EditOptions{
			Content:  *editContent,
			Impetus:  *impetusLabel,
			Date:     dateFlagVal,
			AddRefs:  addRef,
			RmRefs:   rmRef,
			AddBeads: addBead,
			RmBeads:  rmBead,
		})

	case "amend":
		dateFlagVal := *dateStr
		if dateFlagVal == "" {
			dateFlagVal = *dateStrShort
		}
		return humanCLI.Amend(cli.EditOptions{
			Content:  *editContent,
			Impetus:  *impetusLabel,
			Date:     dateFlagVal,
			AddRefs:  addRef,
			RmRefs:   rmRef,
			AddBeads: addBead,
			RmBeads:  rmBead,
		})

	case "redate":
		if len(cmdArgs) < 2 {
			return fmt.Errorf("redate requires beat ID and datetime arguments")
		}
		return humanCLI.Redate(cmdArgs[0], cmdArgs[1])

	case "export":
		exportFs := flag.NewFlagSet("export", flag.ExitOnError)
		exportFormat := exportFs.String("format", "jsonl", "Output format: json, jsonl, csv")
		exportSince := exportFs.String("since", "", "Filter by created_at >= datetime")
		exportUntil := exportFs.String("until", "", "Filter by created_at <= datetime")
		exportImpetus := exportFs.String("impetus", "", "Filter by impetus label (substring match)")
		exportQuery := exportFs.String("query", "", "Filter by content (substring match)")
		exportOutput := exportFs.String("output", "", "Output file (default: stdout)")
		exportOutputShort := exportFs.String("o", "", "Output file (short)")
		if err := exportFs.Parse(cmdArgs); err != nil {
			return err
		}
		output := *exportOutput
		if output == "" {
			output = *exportOutputShort
		}
		return humanCLI.Export(cli.ExportOptions{
			Format:  *exportFormat,
			Since:   *exportSince,
			Until:   *exportUntil,
			Impetus: *exportImpetus,
			Query:   *exportQuery,
			Output:  output,
		})

	case "import":
		importFs := flag.NewFlagSet("import", flag.ExitOnError)
		importFormat := importFs.String("format", "", "Input format: json, jsonl (auto-detect from extension)")
		importOnConflict := importFs.String("on-conflict", "error", "Conflict strategy: error, skip, renumber")
		importSource := importFs.String("source", "", "Set impetus.meta.source on all imported beats")
		importDryRun := importFs.Bool("dry-run", false, "Preview without writing")
		if err := importFs.Parse(cmdArgs); err != nil {
			return err
		}
		importArgs := importFs.Args()
		if len(importArgs) == 0 {
			return fmt.Errorf("import requires file path or - for stdin")
		}
		return humanCLI.Import(importArgs[0], cli.ImportOptions{
			Format:     *importFormat,
			OnConflict: *importOnConflict,
			Source:     *importSource,
			DryRun:     *importDryRun,
		})

	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func handleHooksCommand(beatsDir string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("hooks requires a subcommand: init, status, clear, session-end, configure")
	}

	subcmd := args[0]
	switch subcmd {
	case "init":
		if err := hooks.InitDefaultConfig(beatsDir); err != nil {
			return fmt.Errorf("failed to init hooks: %w", err)
		}
		fmt.Printf("Created hooks config at %s/hooks.json\n", beatsDir)
		fmt.Println("Edit this file to configure synthesis triggers.")
		return nil

	case "status":
		req, err := hooks.GetSynthesisRequest(beatsDir)
		if err != nil {
			fmt.Println("No synthesis pending.")
			return nil
		}
		fmt.Printf("Synthesis triggered at: %s\n", req.TriggeredAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Beats since last synthesis: %d\n", req.BeatsSinceLast)
		fmt.Printf("Total beats: %d\n", req.TotalBeats)
		fmt.Printf("Recent beats to review: %d\n", len(req.RecentBeats))
		fmt.Println("\nUse 'beats hooks clear' after processing, or --robot-synthesis-status for full details.")
		return nil

	case "clear":
		if err := hooks.ClearSynthesisNeeded(beatsDir); err != nil {
			return fmt.Errorf("failed to clear synthesis: %w", err)
		}
		fmt.Println("Synthesis request cleared.")
		return nil

	case "session-end":
		config := hooks.GetSessionEndConfig(beatsDir)
		runner := hooks.NewSessionEndRunner(beatsDir, config)
		return runner.Run()

	case "configure":
		return hooks.ShowConfig(beatsDir)

	default:
		return fmt.Errorf("unknown hooks subcommand: %s (use: init, status, clear, session-end, configure)", subcmd)
	}
}

func printUsage() {
	fmt.Printf(`beats v%s - Narrative substrate for beads
Also available as 'bt' for short.

USAGE:
  bt [command] [arguments]

HUMAN COMMANDS:
  prime                  Output context for AI session injection
  add "content"          Add a new beat with the given content
    --impetus "label"    Optional impetus label
    -d, --date DATE      Backdate beat (ISO8601 or relative: yesterday, 3d ago)
    -w, --web URL        Capture from web URL with title extraction
    -g, --github ref     Capture GitHub repo (owner/repo)
    -x, --twitter URL    Capture X/Twitter link
    -c, --coaching       Mark as coaching insight
    -s, --session-insight Mark as session insight

  list                   List all beats

  show <beat-id>         Show details of a specific beat

  search "query"         Search beats by content/impetus
    --max N              Maximum results (default 20)
    --all                Search across all projects
    --root <path>        Root directory for --all (default: ~/werk or BEATS_ROOT)

  projects               List all beats projects
    --root <path>        Root directory to scan (default: ~/werk or BEATS_ROOT)

  link <beat-id> <bead-id>...  Link a beat to one or more beads

  delete <beat-id>       Delete a beat (alias: rm)
    --force              Skip confirmation prompt

  move <beat-id>         Move a beat to another project (alias: mv)
    --to <directory>     Target .beats directory

  where                  Show which .beats directory is being used

  edit <beat-id>         Edit an existing beat
    --content "text"     Replace content
    --impetus "label"    Replace impetus label
    --date DATE          Change created_at (regenerates ID if date changes)
    --add-ref kind:loc   Add reference
    --rm-ref locator     Remove reference
    --add-bead id        Link bead
    --rm-bead id         Unlink bead

  amend                  Edit most recent beat (same flags as edit)

  redate <id> <date>     Change beat date (convenience for edit --date)

  export                 Export beats to file or stdout
    --format F           Output format: json, jsonl, csv (default: jsonl)
    --since DATE         Filter by created_at >= date
    --until DATE         Filter by created_at <= date
    --impetus "label"    Filter by impetus (substring)
    --query "text"       Filter by content (substring)
    -o, --output FILE    Write to file (default: stdout)

  import <file>          Import beats from JSON/JSONL (use - for stdin)
    --format F           Input format: json, jsonl (auto-detect)
    --on-conflict S      Strategy: error, skip, renumber (default: error)
    --source "label"     Set impetus.meta.source on imported beats
    --dry-run            Preview without writing

  hooks init             Initialize hooks config (enables synthesis triggers)
  hooks status           Check if synthesis is pending
  hooks clear            Clear pending synthesis request

ROBOT COMMANDS (JSON in/out via stdin/stdout):
  --robot-help                   Show robot command schemas
  --robot-propose-beat           Propose beat from raw text
  --robot-commit-beat            Commit a proposed beat
  --robot-search                 Search beats
  --robot-brief                  Generate thematic brief
  --robot-context-for-bead       Get context for a bead
  --robot-map-beats-to-beads     Suggest beat-to-bead mappings
  --robot-diff                   Get changes since timestamp
  --robot-link-beat              Link a beat to beads
  --robot-synthesis-status       Get synthesis status (JSON)
  --robot-synthesis-clear        Clear synthesis request

OPTIONS:
  --dir <path>           Beats directory (default: auto-discover .beats)
  --version              Show version
  --help                 Show this help

DIRECTORY RESOLUTION:
  bt walks up from the current directory to find the nearest .beats folder
  (like git finds .git). Set BEATS_DIR environment variable to override.

EXAMPLES:
  # Add a beat
  bt add "Insight from coaching: commitment is about identity, not discipline"
  bt add --impetus "Research on AI" "Web finding about AI agents"

  # List and show
  bt list
  bt show beat-20251204-001

  # Search
  bt search "coaching"
  bt search --max 5 "commitment"

  # Cross-project search
  bt search --all "deployment"
  bt projects

  # Delete a beat
  bt delete beat-20251204-001
  bt rm --force beat-20251204-001

  # Move beat to another project
  bt move beat-20251204-001 --to /path/to/other/project/.beats

  # See which .beats is being used
  bt where

  # Robot usage (for AI agents)
  echo '{"raw_text":"coaching notes..."}' | bt --robot-propose-beat
  echo '{"content":"...","impetus":{"label":"..."}}' | bt --robot-commit-beat
  echo '{"query":"coaching"}' | bt --robot-search

`, version)
}
