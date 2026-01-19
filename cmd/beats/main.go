package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bierlingm/beats/internal/cli"
	"github.com/bierlingm/beats/internal/hooks"
	"github.com/bierlingm/beats/internal/store"
)

const version = "0.4.1"

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
	default:
		return fmt.Errorf("unknown robot command: %s", cmd)
	}
}

func handleHumanCommand(cmd string, args []string) error {
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
	searchSemantic := fs.Bool("semantic", false, "Use semantic search")
	robotOutput := fs.Bool("robot", false, "Output JSON (for context command)")
	consolidate := fs.Bool("consolidate", false, "Consolidate scattered .beats/ into global store")
	cleanup := fs.Bool("cleanup", false, "Remove old .beats/ directories after migration verification")

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
