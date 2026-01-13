# beats

[![CI](https://github.com/bierlingm/beats/actions/workflows/ci.yml/badge.svg)](https://github.com/bierlingm/beats/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bierlingm/beats)](https://goreportcard.com/report/github.com/bierlingm/beats)
[![Go Reference](https://pkg.go.dev/badge/github.com/bierlingm/beats.svg)](https://pkg.go.dev/github.com/bierlingm/beats)

Narrative substrate for [beads](https://github.com/Dicklesworthstone/beads). Captures insights, discoveries, and reflections that feed into actionable work.

## Philosophy

**Beats** are the "why" and context. **Beads** are the "what" (actionable work).

A beat is a minimally structured narrative unit—an insight from a coaching session, a discovery while browsing, a reflection during a walk. Beats accumulate as raw material. When a beat becomes actionable, it promotes to a bead and the beat remains as narrative context.

```
[beat] "Noticed users struggle with onboarding flow"
   ↓ becomes actionable
[bead] "Redesign onboarding: add progress indicator"
   ↑ beat linked as context
```

## Installation

**One-liner (macOS/Linux):**
```bash
curl -sL https://raw.githubusercontent.com/bierlingm/beats/main/install.sh | sh
```

**With Go:**
```bash
go install github.com/bierlingm/beats/cmd/beats@latest
```

**From source:**
```bash
git clone https://github.com/bierlingm/beats
cd beats && go build -o beats ./cmd/beats
```

## What's New in v0.4

### Smart Impetus Inference
Beats auto-detect impetus from content patterns:
- URLs → "Web discovery", "GitHub discovery", "X discovery"
- Coaching patterns → "Coaching"
- Session patterns → "Session"

### Session Tagging
Beats captured during Droid sessions are auto-tagged:
```bash
# Automatic when FACTORY_SESSION_ID is set
bt add "insight"  # → session_id populated

# Query by session
bt search --session current
bt search --session abc123
```

### Quick Capture Flags
```bash
bt add -w "https://..."       # Web + auto-extract title
bt add -g "owner/repo"        # GitHub + fetch description
bt add -x "https://x.com/..." # X/Twitter
bt add -c "insight"           # Coaching impetus
bt add -s "note"              # Session impetus + auto-tag
```

### Session-End Hooks
```bash
bt hooks session-end   # Create beat from current Factory session
bt hooks configure     # Show hook configuration
```

### Semantic Search
```bash
bt embeddings compute  # Generate embeddings via Ollama
bt embeddings status   # Check coverage
bt search --semantic "concept or pattern"
```

### Shell Alias (Recommended)
```bash
alias ba='bt add'

# Then:
ba "quick insight"
ba -w "https://cool-tool.dev"
```

## Quick Start

```bash
# Add a beat
bt add "Insight from coaching: commitment is about identity, not discipline"
bt add --impetus "Web discovery" "Found interesting tool at https://example.com"

# List and view
bt list
bt show beat-20251211-001

# Search
bt search "coaching"

# Link a beat to beads
bt link beat-20251211-001 mb-abc mb-xyz
```

## Commands

### Human Commands

| Command | Description |
|---------|-------------|
| `beats add "content"` | Add a new beat |
| `beats add --impetus "label" "content"` | Add with custom impetus |
| `beats list` | List all beats |
| `beats show <beat-id>` | Show beat details |
| `beats search "query"` | Search beats |
| `beats link <beat-id> <bead-id>...` | Link beat to beads |

### Robot Commands (JSON via stdin)

For AI agents. All output is JSON to stdout.

```bash
# Commit a beat
echo '{"content":"...","impetus":{"label":"..."}}' | beats --robot-commit-beat

# Search
echo '{"query":"coaching"}' | beats --robot-search

# Link beat to beads
echo '{"beat_id":"beat-20251211-001","bead_ids":["mb-abc"]}' | beats --robot-link-beat

# Get context for a bead
echo '{"bead_id":"mb-abc"}' | beats --robot-context-for-bead

# Full schema
beats --robot-help
```

### Hooks (Synthesis Triggers)

Beats can trigger synthesis when enough accumulate:

```bash
beats hooks init    # Create hooks.json config
beats hooks status  # Check if synthesis pending
beats hooks clear   # Clear after processing
```

Configure `.beats/hooks.json`:
```json
{
  "synthesis": {
    "enabled": true,
    "threshold": 5,
    "action": "file"
  }
}
```

When threshold is reached, `.beats/synthesis_needed.json` is created with a prompt for the "Lattice Weaver" synthesis agent.

## Data Storage

Beats are stored in `.beats/beats.jsonl` (append-only JSONL). Each beat:

```json
{
  "id": "beat-20251211-001",
  "created_at": "2025-12-11T10:30:00Z",
  "updated_at": "2025-12-11T10:30:00Z",
  "impetus": {"label": "Coaching session", "meta": {"channel": "coaching"}},
  "content": "Insight about...",
  "references": [],
  "entities": [],
  "linked_beads": ["mb-abc"]
}
```

## Integration with Beads

```bash
# Create a bead from an insight
bd create "Redesign onboarding flow" -t task -p 2

# Link the originating beat
beats link beat-20251211-001 mb-xyz

# Later, get context for the bead
echo '{"bead_id":"mb-xyz"}' | beats --robot-context-for-bead
```

## License

MIT
