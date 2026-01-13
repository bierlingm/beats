package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func handlePrimeCommand(beatsDir string) error {
	var output strings.Builder
	output.WriteString("# Beats Context\n\n")
	output.WriteString("> Run `bt prime` after new session when .beats/ detected\n\n")

	// Get activating topics
	attention, err := runBtvRobot("--robot-attention", beatsDir)
	if err == nil {
		writeActivatingTopics(&output, attention)
	}

	// Get ripe beats
	ripe, err := runBtvRobot("--robot-ripe", beatsDir)
	if err == nil {
		writeRipeBeats(&output, ripe)
	}

	// Get orientation
	orientation, err := runBtvRobot("--robot-orientation", beatsDir)
	if err == nil {
		writeOrientation(&output, orientation)
	}

	// Quick commands
	output.WriteString("## Quick Commands\n")
	output.WriteString("- `bt add \"insight\"` — capture\n")
	output.WriteString("- `bt add -s \"note\"` — session-tagged\n")
	output.WriteString("- `btv` — launch TUI\n")

	fmt.Print(output.String())
	return nil
}

func runBtvRobot(cmd string, beatsDir string) (map[string]interface{}, error) {
	args := []string{cmd}
	if beatsDir != "" {
		args = append(args, "--dir", beatsDir)
	}

	c := exec.Command("btv", args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("btv %s failed: %w", cmd, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse btv output: %w", err)
	}
	return result, nil
}

func writeActivatingTopics(out *strings.Builder, data map[string]interface{}) {
	activations, ok := data["activations"].([]interface{})
	if !ok || len(activations) == 0 {
		return
	}

	out.WriteString("## Activating Topics (72h)\n")
	for _, a := range activations {
		act, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		cluster := getString(act, "ClusterName")
		count := getInt(act, "BeatCount")
		if cluster == "" || count == 0 {
			continue
		}
		out.WriteString(fmt.Sprintf("- **%s** (%d beats)\n", cluster, count))
	}
	out.WriteString("\n")
}

func writeRipeBeats(out *strings.Builder, data map[string]interface{}) {
	beats, ok := data["beats"].([]interface{})
	if !ok || len(beats) == 0 {
		return
	}

	out.WriteString("## Ripe Beats\n")
	max := 10
	if len(beats) < max {
		max = len(beats)
	}
	for i := 0; i < max; i++ {
		b, ok := beats[i].(map[string]interface{})
		if !ok {
			continue
		}
		id := getString(b, "id")
		preview := getString(b, "preview")
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}
		out.WriteString(fmt.Sprintf("- %s: \"%s\"\n", id, preview))
	}
	out.WriteString("\n")
}

func writeOrientation(out *strings.Builder, data map[string]interface{}) {
	direction := getString(data, "direction")
	summary := getString(data, "summary")
	if direction == "" && summary == "" {
		return
	}

	out.WriteString("## Attention Direction\n")
	if direction != "" {
		out.WriteString(fmt.Sprintf("%s\n", direction))
	}
	if summary != "" {
		out.WriteString(fmt.Sprintf("%s\n", summary))
	}
	out.WriteString("\n")
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
