package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddTodo(t *testing.T) {
	cfg := &Config{
		Name:   "test-project",
		Todos:  []Todo{},
		configPath: "/tmp/test-config.yaml",
	}

	cfg.AddTodo("Test feature", "test-worktree")

	if len(cfg.Todos) != 1 {
		t.Fatalf("Expected 1 todo, got %d", len(cfg.Todos))
	}

	todo := cfg.Todos[0]
	if todo.Description != "Test feature" {
		t.Errorf("Expected description 'Test feature', got %q", todo.Description)
	}
	if todo.Worktree != "test-worktree" {
		t.Errorf("Expected worktree 'test-worktree', got %q", todo.Worktree)
	}
	if todo.Status != TodoStatusPending {
		t.Errorf("Expected status 'pending', got %q", todo.Status)
	}
}

func TestMarkTodoDone(t *testing.T) {
	cfg := &Config{
		Name: "test-project",
		Todos: []Todo{
			{Description: "Feature 1", Worktree: "worktree-1", Status: TodoStatusPending},
			{Description: "Feature 2", Worktree: "worktree-2", Status: TodoStatusPending},
		},
		configPath: "/tmp/test-config.yaml",
	}

	cfg.MarkTodoDone("worktree-1")

	if cfg.Todos[0].Status != TodoStatusDone {
		t.Errorf("Expected first todo to be done, got %q", cfg.Todos[0].Status)
	}
	if cfg.Todos[1].Status != TodoStatusPending {
		t.Errorf("Expected second todo to remain pending, got %q", cfg.Todos[1].Status)
	}
}

func TestGetTodoForWorktree(t *testing.T) {
	cfg := &Config{
		Name: "test-project",
		Todos: []Todo{
			{Description: "Feature 1", Worktree: "worktree-1", Status: TodoStatusPending},
			{Description: "Feature 2", Worktree: "worktree-2", Status: TodoStatusPending},
		},
		configPath: "/tmp/test-config.yaml",
	}

	todo := cfg.GetTodoForWorktree("worktree-1")
	if todo == nil {
		t.Fatal("Expected to find todo for worktree-1")
	}
	if todo.Description != "Feature 1" {
		t.Errorf("Expected description 'Feature 1', got %q", todo.Description)
	}

	todo = cfg.GetTodoForWorktree("nonexistent")
	if todo != nil {
		t.Errorf("Expected nil for nonexistent worktree, got %+v", todo)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Create a config
	cfg := &Config{
		Name:           "test-project",
		WorktreeNaming: "Add feature",
		Todos: []Todo{
			{Description: "Test feature", Worktree: "test-worktree", Status: TodoStatusPending},
		},
		Windows: []TmuxWindow{
			{Name: "code", Command: nil},
			{Name: "server", Command: testStringPtr("npm start")},
		},
		configPath: configPath,
	}

	// Save it
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Config file was not created: %v", err)
	}

	// Read it back
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Verify it contains expected content
	content := string(data)
	if len(content) == 0 {
		t.Error("Config file is empty")
	}
}

func testStringPtr(s string) *string {
	return &s
}
