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

	// If session exists, just attach
	if SessionExists(name) {
		return attachSession(name)
	}

	// Create new session
	return createSession(name, path, cfg)
}

func createSession(name, path string, cfg *config.Config) error {
	// Create initial session (detached)
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Create configured windows
	for i, window := range cfg.Windows {
		windowName := window.Name
		windowPath := path

		if i == 0 {
			// Rename first window instead of creating new one
			cmd = exec.Command("tmux", "rename-window", "-t", fmt.Sprintf("%s:0", name), windowName)
		} else {
			// Create new window
			cmd = exec.Command("tmux", "new-window", "-t", name, "-n", windowName, "-c", windowPath)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create window %s: %w", windowName, err)
		}

		// Run command in window if specified
		if window.Command != nil && *window.Command != "" {
			target := fmt.Sprintf("%s:%s", name, windowName)
			cmd = exec.Command("tmux", "send-keys", "-t", target, *window.Command, "Enter")
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to run command in window %s: %v\n", windowName, err)
			}
		}
	}

	// Select first window
	cmd = exec.Command("tmux", "select-window", "-t", fmt.Sprintf("%s:0", name))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to select first window: %w", err)
	}

	// Attach to session
	return attachSession(name)
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
