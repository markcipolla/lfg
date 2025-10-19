package agent

import (
	"fmt"
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
// It launches Claude Code normally and shows context from previous conversation
func Run(worktreeName string, cfg *config.Config) error {
	// Find the todo for this worktree
	todo := cfg.GetTodoForWorktree(worktreeName)
	if todo == nil {
		// No todo found - just run Claude Code normally
		return runClaudeCode("")
	}

	// Check if we have GitHub integration
	if cfg.StorageBackend == nil || cfg.StorageBackend.Type != "github" {
		// No GitHub integration - just run Claude Code normally
		return runClaudeCode("")
	}

	// Get the issue number from the GitHub URL
	issueNumber, err := extractIssueNumber(todo.GitHubURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to extract issue number: %v\n", err)
		return runClaudeCode("")
	}

	// Load previous conversation from GitHub issue comments
	ctx, err := loadContextFromIssue(cfg, issueNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load context: %v\n", err)
		ctx = ""
	}

	// Run Claude Code with context
	return runClaudeCode(ctx)
}

// runClaudeCode starts Claude Code with optional context
func runClaudeCode(context string) error {
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

	return cmd.Run()
}

// TODO: Implement conversation capture
// For now, conversations are loaded as context but not automatically saved
// Future: Add manual trigger or periodic save functionality

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
