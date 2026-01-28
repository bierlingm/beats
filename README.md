# beats

[![CI](https://github.com/bierlingm/beats/actions/workflows/ci.yml/badge.svg)](https://github.com/bierlingm/beats/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bierlingm/beats)](https://goreportcard.com/report/github.com/bierlingm/beats)
[![Go Reference](https://pkg.go.dev/badge/github.com/bierlingm/beats.svg)](https://pkg.go.dev/github.com/bierlingm/beats)

**Narrative substrate for [beads](https://github.com/Dicklesworthstone/beads).** Captures insights, discoveries, and reflections that feed into actionable work.

## The Mental Model

```
Beats are the "why" — context, insight, narrative
Beads are the "what" — actionable work items

[beat] "Users abandon checkout when shipping costs appear late"
   ↓ crystallizes into action
[bead] "Show shipping estimate on product page"
   ↑ beat remains as context for why this matters
```

A beat is a minimally structured narrative unit: an insight from coaching, a discovery while browsing, a reflection during a walk. Beats accumulate as raw material. When patterns emerge or action becomes clear, a beat promotes to a bead—and the beat remains as context.

## Installation

```bash
# One-liner (macOS/Linux)
curl -sL https://raw.githubusercontent.com/bierlingm/beats/main/install.sh | sh

# With Go
go install github.com/bierlingm/beats/cmd/beats@latest

# From source
git clone https://github.com/bierlingm/beats && cd beats && go install ./cmd/beats
```

**Recommended alias:**
```bash
echo 'alias bt="beats"' >> ~/.zshrc  # or ~/.bashrc
```

## Quick Start

```bash
# Capture an insight
bt add "Noticed users struggle with onboarding flow"

# Backdate a beat you forgot to capture
bt add -d yesterday "Coaching insight: commitment is identity, not discipline"

# Capture from the web
bt add -w "https://interesting-article.com"

# Search your beats
bt search "onboarding"

# Export for backup
bt export -o beats-backup.jsonl
```

---

## Command Reference

### Capturing Beats

```bash
bt add "content"                    # Basic beat
bt add --impetus "Research" "..."   # With custom impetus label
bt add -d "2024-01-15" "..."        # Backdate to specific date
bt add -d "yesterday" "..."         # Backdate with relative date
bt add -d "3d ago" "..."            # 3 days ago
bt add -w "https://..."             # Capture from URL (extracts title)
bt add -g "owner/repo"              # Capture GitHub repo
bt add -x "https://x.com/..."       # Capture X/Twitter post
bt add -c "insight"                 # Mark as coaching insight
bt add -s "note"                    # Mark as session insight
```

### Viewing & Searching

```bash
bt list                             # List all beats
bt show beat-20240115-001           # Show beat details
bt search "query"                   # Search by content/impetus
bt search --max 50 "query"          # Limit results
bt search --all "query"             # Search across all projects
bt search --semantic "concept"      # Semantic search (requires embeddings)
```

### Editing Beats

```bash
bt edit <id> --content "new text"   # Replace content
bt edit <id> --impetus "new label"  # Change impetus
bt edit <id> --date "2024-01-10"    # Change date (regenerates ID)
bt edit <id> --add-bead bd-xyz      # Link to a bead
bt edit <id> --rm-bead bd-xyz       # Unlink from a bead
bt edit <id> --add-ref "url:https://..." # Add reference

bt amend --content "fixed typo"     # Edit most recent beat
bt redate <id> yesterday            # Quick date change
```

### Managing Beats

```bash
bt delete <id>                      # Delete (with confirmation)
bt rm --force <id>                  # Delete without confirmation
bt move <id> --to /path/to/.beats   # Move to another project
bt link <beat-id> <bead-id>...      # Link beat to beads
bt where                            # Show active .beats directory
```

### Import & Export

```bash
# Export
bt export                           # JSONL to stdout
bt export -o backup.jsonl           # To file
bt export --format json             # As JSON array
bt export --format csv              # As CSV
bt export --since 2024-01-01        # Filter by date
bt export --impetus "Coaching"      # Filter by impetus
bt export --query "onboarding"      # Filter by content

# Import
bt import beats.jsonl               # Import from JSONL
bt import data.json --format json   # Import from JSON array
bt import - < piped.jsonl           # Import from stdin
bt import file.jsonl --on-conflict skip     # Skip existing IDs
bt import file.jsonl --on-conflict renumber # Auto-assign new IDs
bt import file.jsonl --source "Migration"   # Tag imported beats
bt import file.jsonl --dry-run      # Preview without writing
```

### Embeddings & Semantic Search

```bash
bt embeddings compute               # Generate embeddings via Ollama
bt embeddings status                # Check embedding coverage
bt search --semantic "concept"      # Use semantic similarity
```

### Hooks & Synthesis

```bash
bt hooks init                       # Initialize hooks config
bt hooks status                     # Check if synthesis pending
bt hooks clear                      # Clear synthesis request
bt hooks session-end                # Trigger session-end hook
```

### Context & Integration

```bash
bt prime                            # Output context for AI injection
bt context [path]                   # Get beats relevant to path
bt projects                         # List all beats projects
```

---

## Robot Commands (for AI Agents)

All robot commands read JSON from stdin and output JSON to stdout.

```bash
# Get full schema
bt --robot-help

# Core operations
echo '{"raw_text":"..."}' | bt --robot-propose-beat
echo '{"content":"...","impetus":{"label":"..."}}' | bt --robot-commit-beat
echo '{"query":"..."}' | bt --robot-search

# Temporal operations (new in v0.5)
echo '{"id":"...", "content":"..."}' | bt --robot-edit
echo '{"content":"..."}' | bt --robot-amend
echo '{"id":"...", "date":"2024-01-15"}' | bt --robot-redate
echo '{"beats":[...], "on_conflict":"renumber"}' | bt --robot-import
echo '{"format":"json", "since":"..."}' | bt --robot-export

# Context & linking
echo '{"bead_id":"..."}' | bt --robot-context-for-bead
echo '{"beat_id":"...", "bead_ids":["..."]}' | bt --robot-link-beat
echo '{}' | bt --robot-map-beats-to-beads
echo '{"since":"2024-01-01T00:00:00Z"}' | bt --robot-diff

# Synthesis
bt --robot-synthesis-status
bt --robot-synthesis-clear
```

---

## Data Model

Beats are stored in `.beats/beats.jsonl` (append-only JSONL):

```json
{
  "id": "beat-20240115-001",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "impetus": {
    "label": "Coaching session",
    "meta": {"channel": "coaching"}
  },
  "content": "Commitment is about identity, not discipline...",
  "references": [
    {"kind": "url", "locator": "https://...", "label": "Source"}
  ],
  "entities": [
    {"label": "onboarding", "category": "concept"}
  ],
  "linked_beads": ["bd-abc", "bd-xyz"],
  "session_id": "factory-session-123",
  "context": {
    "capture_path": "/Users/me/werk/project",
    "wald_directory": "gate/project"
  }
}
```

### ID Format

Beat IDs follow the pattern `beat-YYYYMMDD-NNN`:
- Date portion reflects `created_at`
- Sequence number is unique within the day
- When backdating, IDs are regenerated to match the new date

### Impetus Labels

Impetus captures why a beat was recorded. Auto-inferred from content:
- URLs → "Web discovery", "GitHub discovery", "X discovery"
- Coaching patterns → "Coaching"
- Session markers → "Session"
- Default → "Manual entry"

---

## Configuration

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `BEATS_DIR` | Override beats directory |
| `BEATS_ROOT` | Root for cross-project search |
| `FACTORY_SESSION_ID` | Auto-tag beats with session ID |

### Hooks Configuration

Create `.beats/hooks.json`:

```json
{
  "synthesis": {
    "enabled": true,
    "threshold": 5,
    "action": "file"
  },
  "session_end": {
    "enabled": true,
    "summary_prompt": "Summarize key insights from this session"
  }
}
```

When beat count reaches threshold, `.beats/synthesis_needed.json` is created for processing by synthesis agents.

---

## Integration with Beads

```bash
# Insight captured
bt add "Users abandon checkout when shipping costs surprise them"

# Becomes actionable work
bd create "Show shipping estimate on product page" -t task

# Link the context
bt link beat-20240115-001 bd-xyz

# Later, AI agents can retrieve context
echo '{"bead_id":"bd-xyz"}' | bt --robot-context-for-bead
```

---

## Date Formats

The `--date` / `-d` flag accepts:

| Format | Example |
|--------|---------|
| ISO8601 | `2024-01-15`, `2024-01-15T10:30:00Z` |
| Relative | `yesterday`, `today` |
| Days ago | `3d ago`, `3d`, `-3d` |
| Weeks ago | `2w ago`, `2w`, `1 week ago` |
| Months ago | `1 month ago`, `1m ago` |

---

## Architecture

```
beats/
├── cmd/beats/          # CLI entrypoint
├── internal/
│   ├── beat/           # Core data types
│   ├── cli/            # Human & robot command handlers
│   ├── store/          # JSONL persistence
│   ├── hooks/          # Synthesis triggers
│   ├── capture/        # Web/GitHub/Twitter extraction
│   ├── embeddings/     # Ollama integration
│   └── impetus/        # Auto-inference
└── .beats/             # Data directory
    ├── beats.jsonl     # Beat storage
    ├── hooks.json      # Hook configuration
    └── embeddings.db   # Vector storage
```

---

## License

MIT
