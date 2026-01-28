# Changelog

All notable changes to beats are documented here.

## [0.5.0] - 2026-01-28

### Time Bends to Your Will

This release introduces comprehensive temporal management. Beats are no longer immutable moments frozen in time—you can now backdate, edit, and reshape your narrative substrate.

### Added

#### Backdating (`bt add --date`)
- Capture beats with any timestamp using `-d` / `--date`
- Supports ISO8601: `bt add -d "2024-01-15" "insight"`
- Supports relative dates: `bt add -d yesterday "forgot to capture this"`
- Relative formats: `3d ago`, `2w ago`, `1 month ago`, `today`, `yesterday`
- IDs automatically generated for the target date

#### Edit Command (`bt edit`)
Full beat modification without manual JSONL surgery:
```bash
bt edit <id> --content "corrected text"
bt edit <id> --impetus "Better label"
bt edit <id> --date "2024-01-10"     # Regenerates ID
bt edit <id> --add-bead bd-xyz
bt edit <id> --rm-bead bd-xyz
bt edit <id> --add-ref "url:https://..."
bt edit <id> --rm-ref "https://..."
```

#### Amend Command (`bt amend`)
Quick edit of the most recent beat:
```bash
bt amend --content "fixed typo"
bt amend --impetus "Coaching"
```

#### Redate Command (`bt redate`)
Change only the date (convenience wrapper):
```bash
bt redate beat-20240120-001 yesterday
```

#### Import Command (`bt import`)
Bulk import with conflict resolution:
```bash
bt import beats.jsonl
bt import data.json --format json
bt import - < piped.jsonl               # From stdin
bt import file.jsonl --on-conflict skip      # Skip existing
bt import file.jsonl --on-conflict renumber  # Auto-assign IDs
bt import file.jsonl --source "Migration"    # Tag source
bt import file.jsonl --dry-run               # Preview
```

#### Export Command (`bt export`)
Filtered export in multiple formats:
```bash
bt export                           # JSONL to stdout
bt export -o backup.jsonl           # To file
bt export --format json             # JSON array
bt export --format csv              # CSV
bt export --since 2024-01-01        # Date filter
bt export --until 2024-06-30
bt export --impetus "Coaching"      # Impetus filter
bt export --query "onboarding"      # Content filter
```

#### Robot Commands
Full agent interface for temporal operations:
- `--robot-edit` - Edit by ID
- `--robot-amend` - Edit most recent
- `--robot-redate` - Change date
- `--robot-import` - Bulk import with conflict handling
- `--robot-export` - Filtered export

### Technical Notes

- When backdating or editing dates, the beat ID is regenerated to match the new date
- Sequence numbers are resolved per-date to avoid conflicts
- `updated_at` always reflects the actual modification time
- Edit operations trigger synthesis hooks

---

## [0.4.1] - 2025-12-15

### Fixed
- Lint errors (errcheck, unused, gofmt, misspell)

---

## [0.4.0] - 2025-12-11

### Added
- **Smart Impetus Inference**: Auto-detect impetus from content patterns
- **Session Tagging**: Beats auto-tagged with `FACTORY_SESSION_ID`
- **Quick Capture Flags**: `-w` (web), `-g` (GitHub), `-x` (Twitter), `-c` (coaching), `-s` (session)
- **Session-End Hooks**: `bt hooks session-end` for session summaries
- **Semantic Search**: Via Ollama embeddings (`bt embeddings compute`, `bt search --semantic`)
- **Prime Command**: `bt prime` for AI session context injection

---

## [0.1.0] - 2025-11-01

### Added
- Initial release
- Basic beat capture and listing
- JSONL storage
- Robot command interface
- Beads linking
