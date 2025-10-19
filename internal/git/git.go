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

// GetWorktreePath returns the full path for a worktree by name
func GetWorktreePath(name string) (string, error) {
	worktrees, err := ListWorktrees()
	if err != nil {
		return "", err
	}

	for _, wt := range worktrees {
		if GetWorktreeName(wt.Path) == name {
			return wt.Path, nil
		}
	}

	return "", fmt.Errorf("worktree %q not found", name)
}

// GetCurrentWorktree returns the name of the current worktree, or empty string if not in a worktree
func GetCurrentWorktree() (string, error) {
	// Get the current directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// List all worktrees
	worktrees, err := ListWorktrees()
	if err != nil {
		return "", err
	}

	// Check if current directory is in any worktree
	for _, wt := range worktrees {
		// Check if cwd is the worktree path or a subdirectory of it
		if cwd == wt.Path || strings.HasPrefix(cwd, wt.Path+string(filepath.Separator)) {
			return GetWorktreeName(wt.Path), nil
		}
	}

	return "", nil
}

// CreateWorktree creates a new git worktree in the parent directory of the repo root
func CreateWorktree(name string) error {
	// Get the repository root
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOutput, err := rootCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(rootOutput))

	// Get the parent directory
	parentDir := filepath.Dir(repoRoot)

	// Create worktree path in parent directory
	worktreePath := filepath.Join(parentDir, name)

	// Create branch and worktree
	cmd := exec.Command("git", "worktree", "add", "-b", name, worktreePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %s", string(output))
	}
	return nil
}

// IsBranchMerged checks if a branch has been merged into the default branch
func IsBranchMerged(branchName string) (bool, error) {
	// Get the default branch
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to master/main
		cmd = exec.Command("git", "rev-parse", "--verify", "origin/main")
		if cmd.Run() == nil {
			output = []byte("refs/remotes/origin/main")
		} else {
			output = []byte("refs/remotes/origin/master")
		}
	}
	defaultBranch := strings.TrimSpace(strings.TrimPrefix(string(output), "refs/remotes/"))

	// Check if branch is merged
	cmd = exec.Command("git", "branch", "-r", "--merged", defaultBranch)
	output, err = cmd.Output()
	if err != nil {
		return false, err
	}

	// Look for the branch in the merged list
	mergedBranches := strings.Split(string(output), "\n")
	for _, branch := range mergedBranches {
		branch = strings.TrimSpace(branch)
		if strings.HasSuffix(branch, "/"+branchName) {
			return true, nil
		}
	}

	return false, nil
}

// DeleteWorktree deletes a git worktree
func DeleteWorktree(name string, deleteBranch bool) error {
	// Check if we're currently in the worktree being deleted
	currentWorktree, err := GetCurrentWorktree()
	if err == nil && currentWorktree == name {
		// Change to main worktree first
		mainPath, err := GetMainWorktreePath()
		if err != nil {
			return fmt.Errorf("failed to get main worktree: %w", err)
		}
		if err := os.Chdir(mainPath); err != nil {
			return fmt.Errorf("failed to change to main worktree: %w", err)
		}
	}

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

// GetMainWorktreePath returns the path to the main (non-worktree) repository
func GetMainWorktreePath() (string, error) {
	worktrees, err := ListWorktrees()
	if err != nil {
		return "", err
	}

	// Main worktree has empty branch (it's at the repo root)
	for _, wt := range worktrees {
		if wt.Branch == "" || strings.Contains(wt.Branch, "refs/heads/") && !strings.Contains(wt.Path, "/.git/worktrees/") {
			return wt.Path, nil
		}
	}

	// Fallback: first worktree is usually main
	if len(worktrees) > 0 {
		return worktrees[0].Path, nil
	}

	return "", fmt.Errorf("no main worktree found")
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
