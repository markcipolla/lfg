package tmux

import (
	"fmt"
	"os"
	"os/exec"
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
		if err := ensureWindows(sessionName, path, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to ensure windows: %v\n", err)
		}
		return attachSession(sessionName)
	}

	// Create new session
	return createSession(sessionName, path, cfg)
}

// sanitizeSessionName converts characters that tmux doesn't allow in session names
func sanitizeSessionName(name string) string {
	// Replace dots with underscores (tmux converts dots to underscores)
	return strings.ReplaceAll(name, ".", "_")
}

// ensureWindows checks if the session has the correct pane layout and recreates if needed
func ensureWindows(sessionName, path string, cfg *config.Config) error {
	// Check if the "main" window exists
	cmd := exec.Command("tmux", "list-windows", "-t", sessionName, "-F", "#{window_name}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list windows: %w", err)
	}

	hasMainWindow := false
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "main" {
			hasMainWindow = true
			break
		}
	}

	// If main window doesn't exist, create the pane layout
	if !hasMainWindow {
		// Kill all windows first
		for _, line := range lines {
			if line != "" {
				cmd = exec.Command("tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", sessionName, line))
				cmd.Run() // Ignore errors
			}
		}

		// Create new window with pane layout
		cmd = exec.Command("tmux", "new-window", "-t", sessionName, "-n", "main", "-c", path)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create main window: %w", err)
		}

		// Create the pane layout
		return createPaneLayout(sessionName, path, cfg)
	}

	return nil
}

func createSession(name, path string, cfg *config.Config) error {
	// Verify path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	// Create initial session (detached) with a single window
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create session: %s (output: %s)", err, string(output))
	}

	// Rename the window to "main"
	cmd = exec.Command("tmux", "rename-window", "-t", fmt.Sprintf("%s:0", name), "main")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename window: %w", err)
	}

	return createPaneLayout(name, path, cfg)
}

func createPaneLayout(name, path string, cfg *config.Config) error {
	target := fmt.Sprintf("%s:main", name)

	// Create top pane for description (15% of screen)
	// Split horizontally to create top pane
	cmd := exec.Command("tmux", "split-window", "-t", target, "-v", "-p", "85", "-c", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create top pane: %w", err)
	}

	// The original pane is now at index 0 (top), new pane at index 1 (bottom)
	// Select the bottom pane and split it for the configured panes
	bottomPane := fmt.Sprintf("%s.1", target)

	// Create panes based on configuration
	// We'll create a layout: top description pane, then split bottom into configured panes
	if len(cfg.Windows) > 0 {
		// First configured pane already exists (bottom pane)
		// Run command if specified
		if cfg.Windows[0].Command != nil && *cfg.Windows[0].Command != "" {
			cmd = exec.Command("tmux", "send-keys", "-t", bottomPane, *cfg.Windows[0].Command, "Enter")
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to run command in pane 0: %v\n", err)
			}
		}

		// Create additional panes by splitting
		for i := 1; i < len(cfg.Windows); i++ {
			// Split the current layout to add more panes
			// Use vertical splits for additional panes
			percentage := 100 / (len(cfg.Windows) - i + 1)
			cmd = exec.Command("tmux", "split-window", "-t", bottomPane, "-h", "-p", fmt.Sprintf("%d", percentage), "-c", path)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to create pane %d: %w", i, err)
			}

			// Run command in the new pane if specified
			if cfg.Windows[i].Command != nil && *cfg.Windows[i].Command != "" {
				paneTarget := fmt.Sprintf("%s.%d", target, i+1)
				cmd = exec.Command("tmux", "send-keys", "-t", paneTarget, *cfg.Windows[i].Command, "Enter")
				if err := cmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to run command in pane %d: %v\n", i, err)
				}
			}
		}
	}

	// Setup the description pane (pane 0)
	descPane := fmt.Sprintf("%s.0", target)
	if err := setupDescriptionPane(descPane, name, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to setup description pane: %v\n", err)
	}

	// Select the first work pane (pane 1 - first configured pane)
	cmd = exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s.1", target))
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to select work pane: %v\n", err)
	}

	// Attach to session
	return attachSession(name)
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
