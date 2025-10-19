package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/markcipolla/lfg/internal/config"
	"github.com/markcipolla/lfg/internal/github"
)

// Message represents a single message in the conversation
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // The message content
}

// StreamMessage represents a message in Claude's stream-json format
type StreamMessage struct {
	Type    string  `json:"type"`    // "user_message", "assistant_message", etc.
	Message Message `json:"message"` // The actual message
}

// JSONLEntry represents a single line from Claude's JSONL log file
type JSONLEntry struct {
	Type      string         `json:"type"`      // "user", "assistant", "summary", etc.
	SessionID string         `json:"sessionId"` // Session ID
	Message   MessageContent `json:"message"`   // The actual message
}

// MessageContent represents the message content in the JSONL entry
type MessageContent struct {
	Role    string           `json:"role"`    // "user" or "assistant"
	Content []ContentBlock   `json:"content"` // Content blocks
}

// ContentBlock represents a content block (text, tool use, etc.)
type ContentBlock struct {
	Type string `json:"type"` // "text", "tool_use", etc.
	Text string `json:"text"` // Text content
}

// conversationMonitor monitors the Claude JSONL log and posts to GitHub
type conversationMonitor struct {
	cfg          *config.Config
	issueNumber  int
	projectName  string
	sessionID    string
	lastPosition int64
	stopChan     chan bool
}

// Run starts the agent wrapper for a given worktree
// It launches Claude Code normally and shows context from previous conversation
func Run(worktreeName string, cfg *config.Config) error {
	// Find the todo for this worktree
	todo := cfg.GetTodoForWorktree(worktreeName)
	if todo == nil {
		// No todo found - just run Claude Code normally
		return runClaudeCode("", nil)
	}

	// Check if we have GitHub integration
	if cfg.StorageBackend == nil || cfg.StorageBackend.Type != "github" {
		// No GitHub integration - just run Claude Code normally
		return runClaudeCode("", nil)
	}

	// Get the issue number from the GitHub URL
	issueNumber, err := extractIssueNumber(todo.GitHubURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to extract issue number: %v\n", err)
		return runClaudeCode("", nil)
	}

	// Load previous conversation from GitHub issue comments
	ctx, err := loadContextFromIssue(cfg, issueNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load context: %v\n", err)
		ctx = ""
	}

	// Create conversation monitor
	monitor := &conversationMonitor{
		cfg:         cfg,
		issueNumber: issueNumber,
		stopChan:    make(chan bool),
	}

	// Run Claude Code with context and monitor
	return runClaudeCode(ctx, monitor)
}

// runClaudeCode starts Claude Code with optional context and monitor
func runClaudeCode(context string, monitor *conversationMonitor) error {
	args := []string{"--dangerously-skip-permissions"}

	// If we have context, inject it as a system prompt
	if context != "" {
		args = append(args, "--append-system-prompt", context)
	}

	// Start Claude Code
	cmd := exec.Command("claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// If we have a monitor, start it in the background
	if monitor != nil {
		// Start monitoring in a goroutine
		go monitor.start()
		// Ensure we stop monitoring when Claude exits
		defer monitor.stop()
	}

	return cmd.Run()
}

// start begins monitoring the Claude JSONL log file
func (m *conversationMonitor) start() {
	// Wait a bit for Claude to start and create the session
	time.Sleep(2 * time.Second)

	// Find the most recent Claude session JSONL file
	logPath, err := m.findLatestSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to find Claude session: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Monitoring Claude session log: %s\n", logPath)

	// Monitor the log file
	m.monitorLogFile(logPath)
}

// stop signals the monitor to stop
func (m *conversationMonitor) stop() {
	close(m.stopChan)
}

// findLatestSession finds the most recent Claude session JSONL file
func (m *conversationMonitor) findLatestSession() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Get current working directory and convert to Claude's project name format
	// Claude replaces slashes with hyphens and removes the leading slash
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Convert /Users/foo/bar to -Users-foo-bar
	projectName := strings.ReplaceAll(cwd, "/", "-")
	if strings.HasPrefix(projectName, "-") {
		projectName = projectName[1:] // Remove leading dash
	}

	projectDir := filepath.Join(homeDir, ".claude", "projects", projectName)

	// List all JSONL files in the project directory
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", err
	}

	// Find the most recently modified JSONL file
	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		fullPath := filepath.Join(projectDir, entry.Name())
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFile = fullPath
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("no JSONL files found in %s", projectDir)
	}

	return latestFile, nil
}

// monitorLogFile tails the JSONL log file and processes entries
func (m *conversationMonitor) monitorLogFile(logPath string) {
	file, err := os.Open(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open log file: %v\n", err)
		return
	}
	defer file.Close()

	// Seek to end of file if we have a last position
	if m.lastPosition > 0 {
		file.Seek(m.lastPosition, 0)
	}

	reader := bufio.NewReader(file)

	for {
		select {
		case <-m.stopChan:
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				// No more data, wait a bit and try again
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Update position
			m.lastPosition += int64(len(line))

			// Process the log entry
			m.processLogEntry(line)
		}
	}
}

// processLogEntry parses and processes a single JSONL log entry
func (m *conversationMonitor) processLogEntry(line string) {
	var entry JSONLEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return // Skip invalid JSON
	}

	// Only process user and assistant messages
	if entry.Type != "user" && entry.Type != "assistant" {
		return
	}

	// Extract text content from message.content blocks
	var textParts []string
	for _, block := range entry.Message.Content {
		if block.Type == "text" && block.Text != "" {
			textParts = append(textParts, block.Text)
		}
	}

	if len(textParts) == 0 {
		return // No text content to post
	}

	text := strings.Join(textParts, "\n")

	// Post to GitHub
	var body string
	if entry.Type == "user" {
		body = fmt.Sprintf("**User:** %s", text)
	} else {
		body = fmt.Sprintf("🤖 **Claude:** %s", text)
	}

	err := github.CreateIssueComment(
		m.cfg.StorageBackend.Owner,
		m.cfg.StorageBackend.Repo,
		m.issueNumber,
		body,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to post comment to GitHub: %v\n", err)
	}
}

// loadContextFromIssue loads previous conversation from GitHub issue comments
func loadContextFromIssue(cfg *config.Config, issueNumber int) (string, error) {
	comments, err := github.GetIssueComments(
		cfg.StorageBackend.Owner,
		cfg.StorageBackend.Repo,
		issueNumber,
	)
	if err != nil {
		return "", err
	}

	if len(comments) == 0 {
		return "", nil
	}

	// Build context string from comments
	var ctx strings.Builder
	ctx.WriteString("Previous conversation on this task:\n\n")

	for _, comment := range comments {
		// Determine if this is a user or Claude message
		// We'll use a marker in the comment body to identify Claude's messages
		if strings.HasPrefix(comment.Body, "🤖 **Claude:**") {
			ctx.WriteString(fmt.Sprintf("Assistant: %s\n\n", strings.TrimPrefix(comment.Body, "🤖 **Claude:**")))
		} else {
			ctx.WriteString(fmt.Sprintf("User: %s\n\n", comment.Body))
		}
	}

	return ctx.String(), nil
}

// TODO: postMessageToGitHub - implement manual conversation saving
// For now, users can manually add comments to issues

// extractIssueNumber extracts the issue number from a GitHub URL
// e.g., "https://github.com/owner/repo/issues/123" -> 123
func extractIssueNumber(url string) (int, error) {
	if url == "" {
		return 0, fmt.Errorf("empty GitHub URL")
	}

	// Remove trailing slash if present
	url = strings.TrimSuffix(url, "/")

	// Split by "/"
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid GitHub URL format")
	}

	// Last part should be the issue number
	var issueNum int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &issueNum); err != nil {
		return 0, fmt.Errorf("failed to parse issue number: %w", err)
	}

	return issueNum, nil
}
