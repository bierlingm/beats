package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/moritzbierling/beats/internal/cli"
	"github.com/moritzbierling/beats/internal/store"
)

const version = "0.1.1"

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

	return handleHumanCommand(cmd, cmdArgs)
}

func handleRobotCommand(cmd string, args []string) error {
	// Parse optional --dir flag for robot commands
	robotFlags := flag.NewFlagSet("robot", flag.ExitOnError)
	beatsDir := robotFlags.String("dir", "", "Beats directory")
	robotFlags.Parse(args)

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
		if len(cmdArgs) == 0 {
			return fmt.Errorf("add requires content argument")
		}
		content := strings.Join(cmdArgs, " ")
		return humanCLI.Add(content, *impetusLabel)

	case "list":
		return humanCLI.List()

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
		return humanCLI.Search(query, *maxResults)

	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func printUsage() {
	fmt.Printf(`beats v%s - Narrative substrate for beads

USAGE:
  beats [command] [arguments]

HUMAN COMMANDS:
  add "content"          Add a new beat with the given content
    --impetus "label"    Optional impetus label

  list                   List all beats

  show <beat-id>         Show details of a specific beat

  search "query"         Search beats by content/impetus
    --max N              Maximum results (default 20)

ROBOT COMMANDS (JSON in/out via stdin/stdout):
  --robot-help                   Show robot command schemas
  --robot-propose-beat           Propose beat from raw text
  --robot-commit-beat            Commit a proposed beat
  --robot-search                 Search beats
  --robot-brief                  Generate thematic brief
  --robot-context-for-bead       Get context for a bead
  --robot-map-beats-to-beads     Suggest beat-to-bead mappings
  --robot-diff                   Get changes since timestamp

OPTIONS:
  --dir <path>           Beats directory (default: .beats)
  --version              Show version
  --help                 Show this help

EXAMPLES:
  # Add a beat
  beats add "Insight from coaching: commitment is about identity, not discipline"
  beats add --impetus "Research on AI" "Web finding about AI agents"

  # List and show
  beats list
  beats show beat-20251204-001

  # Search
  beats search "coaching"
  beats search --max 5 "commitment"

  # Robot usage (for AI agents)
  echo '{"raw_text":"coaching notes..."}' | beats --robot-propose-beat
  echo '{"content":"...","impetus":{"label":"..."}}' | beats --robot-commit-beat
  echo '{"query":"coaching"}' | beats --robot-search

`, version)
}
