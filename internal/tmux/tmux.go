package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/markcipolla/lfg/internal/config"
)

// IsInstalled checks if tmux is available
func IsInstalled() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// SessionExists checks if a tmux session exists
func SessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// CreateOrAttachSession creates a new tmux session or attaches to existing one
func CreateOrAttachSession(name, path string, cfg *config.Config) error {
	if !IsInstalled() {
		return fmt.Errorf("tmux is not installed")
	}

	// Sanitize session name - tmux doesn't allow dots in session names
	sessionName := sanitizeSessionName(name)

	// If session exists, ensure windows exist and attach
	if SessionExists(sessionName) {
		if err := ensureWindows(sessionName, name, path, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to ensure windows: %v\n", err)
		}
		return attachSession(sessionName)
	}

	// Create new session (pass both sanitized session name and original worktree name)
	return createSession(sessionName, name, path, cfg)
}

// SanitizeSessionName converts characters that tmux doesn't allow in session names
func SanitizeSessionName(name string) string {
	// Replace dots with underscores (tmux converts dots to underscores)
	return strings.ReplaceAll(name, ".", "_")
}

// sanitizeSessionName is a wrapper for backward compatibility
func sanitizeSessionName(name string) string {
	return SanitizeSessionName(name)
}

// ensureWindows checks if the session has the correct pane layout and recreates if needed
func ensureWindows(sessionName, worktreeName, path string, cfg *config.Config) error {
	// Check if a window with the worktree name exists
	cmd := exec.Command("tmux", "list-windows", "-t", sessionName, "-F", "#{window_name}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list windows: %w", err)
	}

	hasWorktreeWindow := false
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == worktreeName {
			hasWorktreeWindow = true
			break
		}
	}

	// If worktree window doesn't exist, create the pane layout
	if !hasWorktreeWindow {
		// Kill all windows first
		for _, line := range lines {
			if line != "" {
				cmd = exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", sessionName, line))
				cmd.Run() // Ignore errors
			}
		}

		// Create new window with pane layout, named with the worktree name
		cmd = exec.Command("tmux", "new-window", "-t", sessionName, "-n", worktreeName, "-c", path)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create worktree window: %w", err)
		}

		// Create the pane layout
		return createPaneLayout(sessionName, worktreeName, path, cfg)
	}

	return nil
}

func createSession(sessionName, worktreeName, path string, cfg *config.Config) error {
	// Verify path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	// Create initial session (detached) with a single window
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create session: %s (output: %s)", err, string(output))
	}

	// Rename the window to show the worktree name
	cmd = exec.Command("tmux", "rename-window", "-t", fmt.Sprintf("%s:0", sessionName), worktreeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename window: %w", err)
	}

	// Enable mouse mode for this session
	cmd = exec.Command("tmux", "set-option", "-t", sessionName, "mouse", "on")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to enable mouse mode: %v\n", err)
	}

	return createPaneLayout(sessionName, worktreeName, path, cfg)
}

func createPaneLayout(sessionName, worktreeName, path string, cfg *config.Config) error {
	// Use worktree name as the window name
	target := fmt.Sprintf("%s:%s", sessionName, worktreeName)

	// Get layout (handles backward compatibility with old Windows format)
	layout := cfg.GetLayout()
	if len(layout) == 0 {
		return fmt.Errorf("no layout defined in config")
	}

	// Step 1: Create description pane at top (always 5%)
	// Split the initial pane: top 5% for description, bottom 95% for rest
	cmd := exec.Command("tmux", "split-window", "-t", target, "-v", "-p", "95", "-c", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create description pane: %w", err)
	}

	// Now we have:
	// - Pane 0: description (top 5%)
	// - Pane 1: rest (bottom 95%)

	// Setup description pane
	descPane := fmt.Sprintf("%s.0", target)
	if err := setupDescriptionPane(descPane, worktreeName, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to setup description pane: %v\n", err)
	}

	// Step 2: Create agent pane (always 45% of remaining space)
	// Split pane 1: top 45% for agent, bottom 55% for user panes
	cmd = exec.Command("tmux", "split-window", "-t", fmt.Sprintf("%s.1", target), "-v", "-p", "55", "-c", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create agent pane: %w", err)
	}

	// Now we have:
	// - Pane 0: description (top 5%)
	// - Pane 1: agent (45% of remaining 95% = ~43% of screen)
	// - Pane 2: work area (55% of remaining 95% = ~52% of screen)

	// Setup agent pane
	agentPane := fmt.Sprintf("%s.1", target)
	if err := setupAgentPane(agentPane, worktreeName, path, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to setup agent pane: %v\n", err)
	}

	// Step 3: Build work panes in the bottom area according to layout
	// Start with pane 2 (the user-configured work area)
	paneIndex := 2

	// Parse height percentages from layout
	heights := make([]int, len(layout))
	for i, row := range layout {
		height := parsePercentage(row.Height)
		if height <= 0 {
			height = 100 / len(layout) // Default to equal split
		}
		heights[i] = height
	}

	// Track remaining percentage of work area
	remainingPercent := 100

	// Step 1: Create all vertical rows first
	for rowIdx := 1; rowIdx < len(layout); rowIdx++ {
		// Calculate the sum of all remaining rows' heights
		remainingHeight := 0
		for i := rowIdx; i < len(layout); i++ {
			remainingHeight += heights[i]
		}

		// Split percentage: give the new pane all remaining rows' space
		// The current pane will keep what it needs automatically
		splitPercent := (remainingHeight * 100) / remainingPercent

		// Split vertically to create this row (always split the bottom pane)
		splitTarget := fmt.Sprintf("%s.%d", target, paneIndex)
		fmt.Fprintf(os.Stderr, "DEBUG: Creating row %d - splitTarget=%s, paneIndex=%d, splitPercent=%d, remainingPercent=%d, remainingHeight=%d\n",
			rowIdx, splitTarget, paneIndex, splitPercent, remainingPercent, remainingHeight)
		cmd := exec.Command("tmux", "split-window", "-t", splitTarget, "-v", "-p", fmt.Sprintf("%d", splitPercent), "-c", path)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create row %d: %w", rowIdx, err)
		}

		// Update remaining percentage (subtract the row we just created's height)
		remainingPercent -= heights[rowIdx-1]
		paneIndex++
	}

	// Now we have all vertical rows created
	// Pane 0: description
	// Pane 1: agent
	// Pane 2: row 0 (first user row)
	// Pane 3: row 1 (second user row)
	// etc.

	// Step 2: Handle horizontal splits and commands for each row
	paneIndex = 2 // Reset to first user pane (pane 2, after description and agent)
	for rowIdx, row := range layout {
		if len(row.Panes) > 0 {
			// Multi-pane row: split horizontally within this row
			rowStartPane := paneIndex

			// Create all horizontal splits by splitting the leftmost pane each time
			for paneIdx := 1; paneIdx < len(row.Panes); paneIdx++ {
				// Calculate percentage: new pane gets (remaining-1)/remaining of current pane's size
				remainingPanes := len(row.Panes) - paneIdx + 1
				hSplitPercent := (100 * (remainingPanes - 1)) / remainingPanes

				// Always split the first pane of this row (rowStartPane)
				splitTarget := fmt.Sprintf("%s.%d", target, rowStartPane)
				cmd := exec.Command("tmux", "split-window", "-t", splitTarget, "-h", "-p", fmt.Sprintf("%d", hSplitPercent), "-c", path)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to create horizontal pane %d in row %d: %w", paneIdx, rowIdx, err)
				}
			}

			// After all splits, run commands on each pane
			for paneIdx, pane := range row.Panes {
				if pane.Command != nil && *pane.Command != "" {
					paneTarget := fmt.Sprintf("%s.%d", target, rowStartPane+paneIdx)
					cmd := exec.Command("tmux", "send-keys", "-t", paneTarget, *pane.Command, "Enter")
					if err := cmd.Run(); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to run command in pane %s: %v\n", pane.Name, err)
					}
				}
			}

			// Move to next row's starting pane
			paneIndex += len(row.Panes)
		} else {
			// Single-pane row
			if row.Command != nil && *row.Command != "" {
				// Run command if specified
				paneTarget := fmt.Sprintf("%s.%d", target, paneIndex)
				cmd := exec.Command("tmux", "send-keys", "-t", paneTarget, *row.Command, "Enter")
				if err := cmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to run command in pane %s: %v\n", row.Name, err)
				}
			}
			paneIndex++
		}
	}

	// Select the agent pane (pane 1)
	cmd = exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s.1", target))
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to select agent pane: %v\n", err)
	}

	// Attach to session
	return attachSession(sessionName)
}

func setupDescriptionPane(pane, worktreeName string, cfg *config.Config) error {
	// Find lfg binary
	lfgPath := "lfg"

	// Try to find the absolute path
	if absPath, err := exec.LookPath("lfg"); err == nil {
		lfgPath = absPath
	}

	// Get the config path
	configPath := cfg.GetConfigPath()

	// Launch the viewer TUI in the pane using lfg --view with config path
	cmd := exec.Command("tmux", "send-keys", "-t", pane,
		fmt.Sprintf("%s --view --config %s %s", lfgPath, configPath, worktreeName), "Enter")
	return cmd.Run()
}

func setupAgentPane(pane, worktreeName, path string, cfg *config.Config) error {
	// Find lfg binary
	lfgPath := "lfg"

	// Try to find the absolute path
	if absPath, err := exec.LookPath("lfg"); err == nil {
		lfgPath = absPath
	}

	// Get the config path
	configPath := cfg.GetConfigPath()

	// Launch the agent wrapper in the pane
	// The wrapper will handle conversation capture and posting to GitHub
	cmd := exec.Command("tmux", "send-keys", "-t", pane,
		fmt.Sprintf("%s --agent --config %s %s", lfgPath, configPath, worktreeName), "Enter")
	return cmd.Run()
}

func attachSession(name string) error {
	// Check if we're already in a tmux session
	if os.Getenv("TMUX") != "" {
		// Switch to the session
		cmd := exec.Command("tmux", "switch-client", "-t", name)
		return cmd.Run()
	}

	// Attach to session (replace current process)
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// KillSession kills a tmux session
func KillSession(name string) error {
	if !SessionExists(name) {
		return nil
	}

	cmd := exec.Command("tmux", "kill-session", "-t", name)
	return cmd.Run()
}

// ListSessions returns all active tmux sessions
func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// If no sessions exist, tmux returns an error
		if strings.Contains(err.Error(), "no server running") {
			return []string{}, nil
		}
		return nil, err
	}

	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
	return sessions, nil
}

// parsePercentage parses a percentage string like "40%" into an integer 40
func parsePercentage(s string) int {
	// Remove % sign and whitespace
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")

	// Parse as integer
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}
