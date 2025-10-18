package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

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
	if os.Getenv("TMUX") != "" && worktree == "" {
		// We're in tmux and no worktree specified - detach cleanly
		// After detaching, the user will be back at their shell and can run 'lfg' again

		cmd := exec.Command("tmux", "detach-client")
		err := cmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error detaching from tmux: %v\n", err)
			os.Exit(1)
		}

		// Print message that will appear after detaching
		fmt.Println("\n✨ Detached from session. Run 'lfg' again to select a worktree.")
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
	if err := tui.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
