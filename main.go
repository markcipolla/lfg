package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/markcipolla/lfg/internal/config"
	"github.com/markcipolla/lfg/internal/git"
	"github.com/markcipolla/lfg/internal/tui"
	"github.com/markcipolla/lfg/internal/viewer"
)

func main() {
	viewMode := flag.Bool("view", false, "View description for a worktree")
	configPath := flag.String("config", "", "Path to config file (for viewer mode)")
	flag.Parse()

	// Check if worktree name was provided
	worktree := ""
	if flag.NArg() > 0 {
		worktree = flag.Arg(0)
	}

	// View mode: show description viewer
	if *viewMode {
		if worktree == "" {
			fmt.Fprintf(os.Stderr, "Error: --view requires a worktree name\n")
			os.Exit(1)
		}

		// Load config from specified path (viewer doesn't need git repo)
		var cfg *config.Config
		var err error
		if *configPath != "" {
			cfg, err = config.LoadFromPath(*configPath)
		} else {
			cfg, err = config.Load()
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		if err := viewer.Run(worktree, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error running viewer: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Check if we're in a tmux session managed by lfg (before loading config!)
	if os.Getenv("TMUX") != "" && worktree == "" && os.Getenv("LFG_POPUP") == "" {
		// We're in tmux - show the main selector in a popup overlay

		// Find lfg binary
		lfgPath, err := exec.LookPath("lfg")
		if err != nil {
			lfgPath = "lfg"
		}

		// Get the main repo root (where we want to run lfg from)
		// Try to get the main worktree root by listing all worktrees
		cmd := exec.Command("git", "worktree", "list", "--porcelain")
		output, err := cmd.Output()
		var repoRootStr string
		if err == nil {
			// Parse the output to get the first worktree path (main worktree)
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "worktree ") {
					repoRootStr = strings.TrimPrefix(line, "worktree ")
					break
				}
			}
		}
		if repoRootStr == "" {
			// Fallback to rev-parse if worktree list fails
			cmd = exec.Command("git", "rev-parse", "--show-toplevel")
			repoRoot, err := cmd.Output()
			if err == nil && len(repoRoot) > 0 {
				repoRootStr = string(repoRoot[:len(repoRoot)-1]) // trim newline
			} else {
				// If not in a git repo, just use current dir
				repoRootStr, _ = os.Getwd()
			}
		}

		// Use tmux display-popup to show lfg in a fullscreen popup
		// When they exit the popup, they're back in the current pane
		popupCmd := fmt.Sprintf("cd '%s' && LFG_POPUP=1 %s", repoRootStr, lfgPath)
		cmd = exec.Command("tmux", "display-popup", "-E", "-w", "100%", "-h", "100%", popupCmd)
		cmd.Run() // Ignore errors

		os.Exit(0)
	}

	// Load config (creates default if missing)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// If worktree specified, jump directly to it
	if worktree != "" {
		if err := git.JumpToWorktree(worktree, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error jumping to worktree: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise, show TUI
	result, err := tui.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	// Handle the result
	if result != nil && result.SelectedWorktree != "" {
		// If user wants to exit to main, handle specially
		if result.ExitToMain {
			// Get the main worktree path
			worktrees, err := git.ListWorktrees()
			if err == nil && len(worktrees) > 0 {
				mainPath := worktrees[0].Path

				// If we're in a tmux session, send commands to cd and detach
				if os.Getenv("TMUX") != "" {
					// Get current session name
					sessionName := ""
					cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}")
					if output, err := cmd.Output(); err == nil {
						sessionName = strings.TrimSpace(string(output))
					}

					if sessionName != "" {
						// Send command to cd to main path and then kill the session
						// This will happen after the popup closes
						cdCmd := fmt.Sprintf("cd '%s' && tmux kill-session", mainPath)
						exec.Command("tmux", "send-keys", "-t", sessionName, cdCmd, "Enter").Run()
					}
				} else {
					// Not in tmux, just cd
					os.Chdir(mainPath)
				}
			}
			return
		}

		// Otherwise, jump to the selected worktree
		if err := git.JumpToWorktree(result.SelectedWorktree, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error jumping to worktree: %v\n", err)
			os.Exit(1)
		}
	}
}
