package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/markcipolla/lfg/internal/config"
	"github.com/markcipolla/lfg/internal/tmux"
)

type Worktree struct {
	Path   string
	Branch string
	Commit string
}

// ListWorktrees returns all git worktrees
func ListWorktrees() ([]Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktrees []Worktree
	lines := strings.Split(string(output), "\n")

	var current Worktree
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		}
	}

	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// GetWorktreeName extracts the worktree name from its path
func GetWorktreeName(path string) string {
	return filepath.Base(path)
}

// CreateWorktree creates a new git worktree
func CreateWorktree(name string) error {
	// Create branch and worktree
	cmd := exec.Command("git", "worktree", "add", "-b", name, name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s", string(output))
	}
	return nil
}

// DeleteWorktree deletes a git worktree
func DeleteWorktree(name string, deleteBranch bool) error {
	// Remove worktree
	cmd := exec.Command("git", "worktree", "remove", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %s", string(output))
	}

	// Delete branch if requested
	if deleteBranch {
		cmd = exec.Command("git", "branch", "-D", name)
		if err := cmd.Run(); err != nil {
			// Don't fail if branch deletion fails
			fmt.Fprintf(os.Stderr, "Warning: failed to delete branch %s\n", name)
		}
	}

	return nil
}

// JumpToWorktree switches to a worktree by creating/attaching tmux session
func JumpToWorktree(name string, cfg *config.Config) error {
	// Find worktree
	worktrees, err := ListWorktrees()
	if err != nil {
		return err
	}

	var targetPath string
	for _, wt := range worktrees {
		if GetWorktreeName(wt.Path) == name {
			targetPath = wt.Path
			break
		}
	}

	if targetPath == "" {
		return fmt.Errorf("worktree '%s' not found", name)
	}

	// Create/attach tmux session
	return tmux.CreateOrAttachSession(name, targetPath, cfg)
}
