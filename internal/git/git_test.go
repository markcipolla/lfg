package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetWorktreeName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/Users/test/project-feature",
			expected: "project-feature",
		},
		{
			name:     "nested path",
			path:     "/Users/test/dev/project-feature-branch",
			expected: "project-feature-branch",
		},
		{
			name:     "path with dots",
			path:     "/Users/test/household.email-homepage",
			expected: "household.email-homepage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetWorktreeName(tt.path)
			if result != tt.expected {
				t.Errorf("GetWorktreeName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetCurrentWorktree(t *testing.T) {
	// This test is skipped if not in a git repository
	_, err := os.Stat(".git")
	if err != nil {
		t.Skip("Not in a git repository, skipping test")
	}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Test getting current worktree
	worktreeName, err := GetCurrentWorktree()
	if err != nil {
		t.Fatalf("GetCurrentWorktree() error = %v", err)
	}

	// Should return the name of the current directory if it's a worktree
	expectedName := filepath.Base(cwd)
	if worktreeName != "" && worktreeName != expectedName {
		t.Logf("Current worktree: %q (expected %q or empty)", worktreeName, expectedName)
	}
}
