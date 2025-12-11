package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bierlingm/beats/internal/beat"
)

const (
	HooksConfigFile = "hooks.json"
	HookStateFile   = "hook_state.json"
	SynthesisFile   = "synthesis_needed.json"
)

// HooksConfig defines hook triggers and actions.
type HooksConfig struct {
	Synthesis SynthesisHook `json:"synthesis"`
}

// SynthesisHook configures when synthesis should be triggered.
type SynthesisHook struct {
	Enabled   bool   `json:"enabled"`
	Threshold int    `json:"threshold"` // Number of beats between syntheses
	Action    string `json:"action"`    // "file" or "script"
	Script    string `json:"script"`    // Path to script (if action is "script")
}

// HookState tracks hook execution state.
type HookState struct {
	LastSynthesisAt    time.Time `json:"last_synthesis_at"`
	LastSynthesisCount int       `json:"last_synthesis_count"`
	TotalBeats         int       `json:"total_beats"`
}

// SynthesisRequest is written to synthesis_needed.json when triggered.
type SynthesisRequest struct {
	TriggeredAt     time.Time   `json:"triggered_at"`
	BeatsSinceLast  int         `json:"beats_since_last"`
	TotalBeats      int         `json:"total_beats"`
	RecentBeats     []beat.Beat `json:"recent_beats"`
	SynthesisPrompt string      `json:"synthesis_prompt"`
}

// Manager handles hook execution.
type Manager struct {
	beatsDir string
	config   *HooksConfig
	state    *HookState
}

// NewManager creates a new hooks manager.
func NewManager(beatsDir string) (*Manager, error) {
	m := &Manager{beatsDir: beatsDir}

	if err := m.loadConfig(); err != nil {
		// No config file = hooks disabled, not an error
		m.config = &HooksConfig{
			Synthesis: SynthesisHook{Enabled: false},
		}
	}

	if err := m.loadState(); err != nil {
		m.state = &HookState{}
	}

	return m, nil
}

func (m *Manager) loadConfig() error {
	path := filepath.Join(m.beatsDir, HooksConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	m.config = &HooksConfig{}
	return json.Unmarshal(data, m.config)
}

func (m *Manager) loadState() error {
	path := filepath.Join(m.beatsDir, HookStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	m.state = &HookState{}
	return json.Unmarshal(data, m.state)
}

func (m *Manager) saveState() error {
	path := filepath.Join(m.beatsDir, HookStateFile)
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// OnBeatAdded is called after a beat is successfully added.
// It checks if any hooks should be triggered.
func (m *Manager) OnBeatAdded(newBeat *beat.Beat, allBeats []beat.Beat) error {
	m.state.TotalBeats = len(allBeats)

	if err := m.checkSynthesisHook(allBeats); err != nil {
		return fmt.Errorf("synthesis hook failed: %w", err)
	}

	return m.saveState()
}

func (m *Manager) checkSynthesisHook(allBeats []beat.Beat) error {
	if !m.config.Synthesis.Enabled {
		return nil
	}

	threshold := m.config.Synthesis.Threshold
	if threshold <= 0 {
		threshold = 5 // Default to 5 beats
	}

	beatsSinceLast := m.state.TotalBeats - m.state.LastSynthesisCount
	if beatsSinceLast < threshold {
		return nil
	}

	// Threshold reached - trigger synthesis
	return m.triggerSynthesis(allBeats, beatsSinceLast)
}

func (m *Manager) triggerSynthesis(allBeats []beat.Beat, beatsSinceLast int) error {
	// Get recent beats (since last synthesis)
	var recentBeats []beat.Beat
	if m.state.LastSynthesisAt.IsZero() {
		recentBeats = allBeats
	} else {
		for _, b := range allBeats {
			if b.CreatedAt.After(m.state.LastSynthesisAt) {
				recentBeats = append(recentBeats, b)
			}
		}
	}

	request := SynthesisRequest{
		TriggeredAt:     time.Now().UTC(),
		BeatsSinceLast:  beatsSinceLast,
		TotalBeats:      m.state.TotalBeats,
		RecentBeats:     recentBeats,
		SynthesisPrompt: generateSynthesisPrompt(recentBeats),
	}

	switch m.config.Synthesis.Action {
	case "script":
		if err := m.runScript(request); err != nil {
			return err
		}
	default: // "file" or empty
		if err := m.writeSynthesisFile(request); err != nil {
			return err
		}
	}

	// Update state
	m.state.LastSynthesisAt = time.Now().UTC()
	m.state.LastSynthesisCount = m.state.TotalBeats

	return nil
}

func (m *Manager) writeSynthesisFile(request SynthesisRequest) error {
	path := filepath.Join(m.beatsDir, SynthesisFile)
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m *Manager) runScript(request SynthesisRequest) error {
	if m.config.Synthesis.Script == "" {
		return fmt.Errorf("script path not configured")
	}

	// Write request to temp file for script to read
	tempFile := filepath.Join(m.beatsDir, "synthesis_request_temp.json")
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	cmd := exec.Command(m.config.Synthesis.Script, tempFile)
	cmd.Dir = m.beatsDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script failed: %w\nOutput: %s", err, string(output))
	}

	// Clean up temp file
	os.Remove(tempFile)
	return nil
}

func generateSynthesisPrompt(recentBeats []beat.Beat) string {
	var beatSummaries []string
	for _, b := range recentBeats {
		summary := fmt.Sprintf("- [%s] %s: %s", b.ID, b.Impetus.Label, truncate(b.Content, 100))
		beatSummaries = append(beatSummaries, summary)
	}

	prompt := fmt.Sprintf(`You are the Lattice Weaver - a synthesis agent for the beats/beads system.

%d new beats have accumulated since the last synthesis. Review them and help "close loops" and "weave things together":

RECENT BEATS:
%s

YOUR TASK:
1. CLUSTERS: Identify thematic clusters or patterns among these beats
2. CONNECTIONS: Suggest links between beats that relate to each other
3. OPEN LOOPS: Identify beats that mention something unresolved or that should be followed up
4. BEAD CANDIDATES: Propose any actionable items (beads) that emerge from these beats
5. LINKS TO EXISTING: If you know of existing beads, suggest which beats should link to them

Output a concise synthesis report that helps maintain coherence across the knowledge substrate.`,
		len(recentBeats),
		joinStrings(beatSummaries, "\n"),
	)

	return prompt
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// InitDefaultConfig creates a default hooks.json if it doesn't exist.
func InitDefaultConfig(beatsDir string) error {
	path := filepath.Join(beatsDir, HooksConfigFile)
	if _, err := os.Stat(path); err == nil {
		return nil // Already exists
	}

	config := HooksConfig{
		Synthesis: SynthesisHook{
			Enabled:   true,
			Threshold: 5,
			Action:    "file",
			Script:    "",
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ClearSynthesisNeeded removes the synthesis_needed.json file (call after processing).
func ClearSynthesisNeeded(beatsDir string) error {
	path := filepath.Join(beatsDir, SynthesisFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GetSynthesisRequest reads the current synthesis request if one exists.
func GetSynthesisRequest(beatsDir string) (*SynthesisRequest, error) {
	path := filepath.Join(beatsDir, SynthesisFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var req SynthesisRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
}
