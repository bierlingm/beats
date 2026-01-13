package hooks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bierlingm/beats/internal/beat"
)

// SessionEndHook configures session-end beat creation
type SessionEndHook struct {
	Enabled       bool   `json:"enabled"`
	OllamaModel   string `json:"ollama_model"`
	OllamaURL     string `json:"ollama_url"`
	MinMessages   int    `json:"min_messages"`
	MaxContentLen int    `json:"max_content_len"`
	ProcessedFile string `json:"processed_file"`
}

// DefaultSessionEndHook returns sensible defaults
func DefaultSessionEndHook() SessionEndHook {
	return SessionEndHook{
		Enabled:       true,
		OllamaModel:   "mistral:latest",
		OllamaURL:     "http://localhost:11434",
		MinMessages:   5,
		MaxContentLen: 500,
		ProcessedFile: filepath.Join(os.Getenv("HOME"), ".factory/.processed-session-beats"),
	}
}

// FactorySession represents a Factory/Droid session file
type FactorySession struct {
	ID       string
	Title    string
	FilePath string
	Messages []SessionMessage
}

// SessionMessage represents a message from a Factory session
type SessionMessage struct {
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// SessionEndRunner handles session-end beat creation
type SessionEndRunner struct {
	config     SessionEndHook
	beatsDir   string
	httpClient *http.Client
}

// NewSessionEndRunner creates a new runner
func NewSessionEndRunner(beatsDir string, config SessionEndHook) *SessionEndRunner {
	return &SessionEndRunner{
		config:   config,
		beatsDir: beatsDir,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Run executes the session-end hook
func (r *SessionEndRunner) Run() error {
	if !r.config.Enabled {
		return fmt.Errorf("session-end hook is disabled")
	}

	session, err := r.findCurrentSession()
	if err != nil {
		return fmt.Errorf("finding session: %w", err)
	}

	if r.isProcessed(session.ID) {
		fmt.Printf("Session %s already processed\n", session.ID)
		return nil
	}

	if len(session.Messages) < r.config.MinMessages {
		fmt.Printf("Session has %d messages (min: %d), skipping\n", len(session.Messages), r.config.MinMessages)
		return nil
	}

	content := r.extractContent(session)
	if content == "" {
		return fmt.Errorf("no content extracted from session")
	}

	summary, err := r.generateSummary(content)
	if err != nil {
		return fmt.Errorf("generating summary: %w", err)
	}

	if summary == "" {
		return fmt.Errorf("empty summary generated")
	}

	b := &beat.Beat{
		ID:        beat.GenerateIDWithSequence(time.Now().UTC(), 1),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		SessionID: session.ID,
		Impetus: beat.Impetus{
			Label: "Session",
			Meta: map[string]string{
				"session_id": session.ID,
				"title":      session.Title,
			},
		},
		Content:     summary,
		References:  []beat.Reference{},
		Entities:    []beat.Entity{},
		LinkedBeads: []string{},
	}

	// Write directly to JSONL to avoid import cycle
	if err := r.appendBeat(b); err != nil {
		return fmt.Errorf("saving beat: %w", err)
	}

	r.markProcessed(session.ID)

	fmt.Printf("Created beat %s from session: %s\n", b.ID, session.Title)
	return nil
}

func (r *SessionEndRunner) findCurrentSession() (*FactorySession, error) {
	sessionsDir := filepath.Join(os.Getenv("HOME"), ".factory/sessions")

	// Get CWD-specific session directory
	cwd, _ := os.Getwd()
	cwdEncoded := strings.ReplaceAll(cwd, "/", "-")
	if strings.HasPrefix(cwdEncoded, "-") {
		cwdEncoded = cwdEncoded[1:]
	}
	sessionDir := filepath.Join(sessionsDir, cwdEncoded)

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		sessionDir = sessionsDir
	}

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("reading sessions directory: %w", err)
	}

	var newest string
	var newestTime time.Time

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newest = filepath.Join(sessionDir, e.Name())
		}
	}

	if newest == "" {
		return nil, fmt.Errorf("no session files found in %s", sessionDir)
	}

	return r.parseSession(newest)
}

func (r *SessionEndRunner) parseSession(path string) (*FactorySession, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	session := &FactorySession{
		ID:       strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		FilePath: path,
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	isFirst := true
	for scanner.Scan() {
		line := scanner.Bytes()

		if isFirst {
			var meta struct {
				Title string `json:"title"`
			}
			json.Unmarshal(line, &meta)
			session.Title = meta.Title
			isFirst = false
		}

		var msg SessionMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.Type == "message" && msg.Message.Role == "user" {
			session.Messages = append(session.Messages, msg)
		}
	}

	if session.Title == "" {
		session.Title = session.ID
	}

	return session, scanner.Err()
}

func (r *SessionEndRunner) extractContent(session *FactorySession) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Session: %s", session.Title))
	parts = append(parts, "")
	parts = append(parts, "User messages:")

	for _, msg := range session.Messages {
		for _, content := range msg.Message.Content {
			if content.Type != "text" {
				continue
			}
			text := strings.TrimSpace(content.Text)
			// Skip system messages
			if strings.HasPrefix(text, "<") || strings.Contains(text, "IMPORTANT:") {
				continue
			}
			// Skip very short
			if len(text) < 5 {
				continue
			}
			// Truncate very long messages
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			parts = append(parts, "- "+text)
		}
	}

	return strings.Join(parts, "\n")
}

func (r *SessionEndRunner) generateSummary(content string) (string, error) {
	prompt := fmt.Sprintf(`Summarize this coding/terminal session as a concise technical insight or learning (1-2 sentences). Focus on what was discovered, built, or solved. No fluff, be specific:

%s`, content)

	reqBody := map[string]interface{}{
		"model":  r.config.OllamaModel,
		"prompt": prompt,
		"stream": false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Post(
		r.config.OllamaURL+"/api/generate",
		"application/json",
		strings.NewReader(string(jsonBody)),
	)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	summary := strings.TrimSpace(result.Response)
	if len(summary) > r.config.MaxContentLen {
		summary = summary[:r.config.MaxContentLen]
	}

	return summary, nil
}

func (r *SessionEndRunner) isProcessed(sessionID string) bool {
	data, err := os.ReadFile(r.config.ProcessedFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), sessionID)
}

func (r *SessionEndRunner) markProcessed(sessionID string) {
	dir := filepath.Dir(r.config.ProcessedFile)
	os.MkdirAll(dir, 0755)

	f, err := os.OpenFile(r.config.ProcessedFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(sessionID + "\n")
}

// appendBeat writes a beat directly to the JSONL file (avoids import cycle with store)
func (r *SessionEndRunner) appendBeat(b *beat.Beat) error {
	beatsFile := filepath.Join(r.beatsDir, "beats.jsonl")
	
	// Ensure directory exists
	if err := os.MkdirAll(r.beatsDir, 0755); err != nil {
		return err
	}
	
	f, err := os.OpenFile(beatsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	
	_, err = f.Write(append(data, '\n'))
	return err
}

// GetSessionEndConfig reads config or returns defaults
func GetSessionEndConfig(beatsDir string) SessionEndHook {
	path := filepath.Join(beatsDir, HooksConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultSessionEndHook()
	}

	var fullConfig struct {
		SessionEnd SessionEndHook `json:"session_end"`
	}
	if err := json.Unmarshal(data, &fullConfig); err != nil {
		return DefaultSessionEndHook()
	}

	config := fullConfig.SessionEnd
	if config.OllamaModel == "" {
		config.OllamaModel = DefaultSessionEndHook().OllamaModel
	}
	if config.OllamaURL == "" {
		config.OllamaURL = DefaultSessionEndHook().OllamaURL
	}
	if config.MinMessages == 0 {
		config.MinMessages = DefaultSessionEndHook().MinMessages
	}
	if config.MaxContentLen == 0 {
		config.MaxContentLen = DefaultSessionEndHook().MaxContentLen
	}
	if config.ProcessedFile == "" {
		config.ProcessedFile = DefaultSessionEndHook().ProcessedFile
	}

	return config
}

// ShowConfig displays current hooks configuration
func ShowConfig(beatsDir string) error {
	config := struct {
		Synthesis  SynthesisHook  `json:"synthesis"`
		SessionEnd SessionEndHook `json:"session_end"`
	}{
		SessionEnd: GetSessionEndConfig(beatsDir),
	}

	// Load synthesis config
	mgr, err := NewManager(beatsDir)
	if err == nil {
		config.Synthesis = mgr.config.Synthesis
	}

	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println("Current hooks configuration:")
	fmt.Println(string(configJSON))
	fmt.Printf("\nConfig file: %s/%s\n", beatsDir, HooksConfigFile)
	return nil
}
