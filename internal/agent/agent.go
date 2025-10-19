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
	"github.com/markcipolla/lfg/internal/git"
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
	Role    string          `json:"role"`    // "user" or "assistant"
	Content json.RawMessage `json:"content"` // Can be string (user) or array (assistant)
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
	worktreePath string // Full path to the worktree directory
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

	// Get the worktree path
	worktreePath, err := git.GetWorktreePath(worktreeName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get worktree path: %v\n", err)
		return runClaudeCode(ctx, nil)
	}

	// Create conversation monitor
	monitor := &conversationMonitor{
		cfg:          cfg,
		issueNumber:  issueNumber,
		worktreePath: worktreePath,
		stopChan:     make(chan bool),
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
	// Wait for Claude to create a session (up to 30 seconds)
	var logPath string
	var err error

	// Check more frequently - every 100ms
	for i := 0; i < 300; i++ {
		time.Sleep(100 * time.Millisecond)

		logPath, err = m.findLatestSession()
		if err == nil {
			break
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to find Claude session after 30s: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Monitoring Claude session log: %s\n", logPath)

	// Don't seek to end - monitor from beginning to catch all messages
	m.lastPosition = 0

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

	// Convert worktree path to Claude's project name format
	// Claude replaces slashes and dots with hyphens:
	// /Users/foo/bar.baz -> -Users-foo-bar-baz
	projectName := strings.ReplaceAll(m.worktreePath, "/", "-")
	projectName = strings.ReplaceAll(projectName, ".", "-")

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
	fmt.Fprintf(os.Stderr, "[DEBUG] Starting monitoring at position %d\n", m.lastPosition)

	for {
		select {
		case <-m.stopChan:
			return
		default:
			// Open file each time to pick up new data
			file, err := os.Open(logPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to open log file: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// Seek to last position
			file.Seek(m.lastPosition, 0)
			reader := bufio.NewReader(file)

			// Read all available lines
			linesRead := 0
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					// No more data available
					break
				}

				linesRead++
				m.lastPosition += int64(len(line))
				m.processLogEntry(line)
			}

			file.Close()

			if linesRead > 0 {
				fmt.Fprintf(os.Stderr, "[DEBUG] Read %d new lines\n", linesRead)
			}

			// Wait before checking for more data
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// processLogEntry parses and processes a single JSONL log entry
func (m *conversationMonitor) processLogEntry(line string) {
	fmt.Fprintf(os.Stderr, "[DEBUG] Processing log entry\n")

	var entry JSONLEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] Failed to parse JSON: %v\n", err)
		return // Skip invalid JSON
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Entry type: %s\n", entry.Type)

	// Only process user and assistant messages
	if entry.Type != "user" && entry.Type != "assistant" {
		return
	}

	// Extract text content - handle both string (user) and array (assistant) formats
	var text string

	// Try parsing as a string first (user messages)
	if err := json.Unmarshal(entry.Message.Content, &text); err == nil && text != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] Parsed as string: %s\n", text)
		// Successfully parsed as string
	} else {
		// Try parsing as array of content blocks (assistant messages)
		var blocks []ContentBlock
		if err := json.Unmarshal(entry.Message.Content, &blocks); err == nil {
			var textParts []string
			for _, block := range blocks {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
			text = strings.Join(textParts, "\n")
			fmt.Fprintf(os.Stderr, "[DEBUG] Parsed as array: %s\n", text)
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] Failed to parse content: %v\n", err)
		}
	}

	if text == "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] No text content found\n")
		return // No text content to post
	}

	// Post to GitHub
	var body string
	if entry.Type == "user" {
		body = fmt.Sprintf("**User:** %s", text)
	} else {
		body = fmt.Sprintf("🤖 **Claude:** %s", text)
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Posting to GitHub issue %d: %s\n", m.issueNumber, body)

	err := github.CreateIssueComment(
		m.cfg.StorageBackend.Owner,
		m.cfg.StorageBackend.Repo,
		m.issueNumber,
		body,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to post comment to GitHub: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "[DEBUG] Successfully posted to GitHub\n")
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
