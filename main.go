package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/markcipolla/lfg/internal/config"
	"github.com/markcipolla/lfg/internal/git"
	"github.com/markcipolla/lfg/internal/tui"
)

func main() {
	flag.Parse()

	// Check if worktree name was provided
	worktree := ""
	if flag.NArg() > 0 {
		worktree = flag.Arg(0)
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
