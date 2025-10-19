package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

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

// Run starts the agent wrapper for a given worktree
// It launches Claude Code with stream-json format and captures the conversation
func Run(worktreeName string, cfg *config.Config) error {
	// Find the todo for this worktree
	todo := cfg.GetTodoForWorktree(worktreeName)
	if todo == nil {
		return fmt.Errorf("no todo found for worktree: %s", worktreeName)
	}

	// Check if we have GitHub integration
	if cfg.StorageBackend == nil || cfg.StorageBackend.Type != "github" {
		return fmt.Errorf("GitHub integration required for agent mode")
	}

	// Get the issue number from the GitHub URL
	issueNumber, err := extractIssueNumber(todo.GitHubURL)
	if err != nil {
		return fmt.Errorf("failed to extract issue number: %w", err)
	}

	// Load previous conversation from GitHub issue comments
	ctx, err := loadContextFromIssue(cfg, issueNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load context: %v\n", err)
	}

	// Start Claude Code with stream-json format
	cmd := exec.Command("claude",
		"--dangerously-skip-permissions",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--replay-user-messages",
	)

	// If we have context, inject it as a system prompt
	if ctx != "" {
		cmd.Args = append(cmd.Args, "--append-system-prompt", ctx)
	}

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start claude: %w", err)
	}

	// Create channels for coordinating goroutines
	done := make(chan error, 3)

	// Goroutine to copy user input to Claude
	go func() {
		_, err := io.Copy(stdin, os.Stdin)
		stdin.Close()
		done <- err
	}()

	// Goroutine to handle Claude's output
	go func() {
		done <- handleClaudeOutput(stdout, cfg, issueNumber)
	}()

	// Goroutine to copy stderr to our stderr
	go func() {
		_, err := io.Copy(os.Stderr, stderr)
		done <- err
	}()

	// Wait for command to finish
	cmdErr := cmd.Wait()

	// Wait for goroutines to finish
	for i := 0; i < 3; i++ {
		if err := <-done; err != nil && cmdErr == nil {
			cmdErr = err
		}
	}

	return cmdErr
}

// handleClaudeOutput reads Claude's stream-json output, displays it to user, and posts to GitHub
func handleClaudeOutput(reader io.Reader, cfg *config.Config, issueNumber int) error {
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()

		// Write the line to stdout for the user to see
		fmt.Println(line)

		// Parse the JSON to extract messages
		var msg StreamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Not a valid JSON message, skip
			continue
		}

		// Post message to GitHub if it's a user or assistant message
		if msg.Type == "user_message" || msg.Type == "assistant_message" {
			if err := postMessageToGitHub(cfg, issueNumber, msg.Message); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to post message to GitHub: %v\n", err)
			}
		}
	}

	return scanner.Err()
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

// postMessageToGitHub posts a message as a comment on the GitHub issue
func postMessageToGitHub(cfg *config.Config, issueNumber int, msg Message) error {
	var body string

	if msg.Role == "assistant" {
		// Mark Claude's messages with a robot emoji
		body = fmt.Sprintf("🤖 **Claude:**\n\n%s", msg.Content)
	} else {
		// User messages go as-is
		body = msg.Content
	}

	return github.CreateIssueComment(
		cfg.StorageBackend.Owner,
		cfg.StorageBackend.Repo,
		issueNumber,
		body,
	)
}

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
