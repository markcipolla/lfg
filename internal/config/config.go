package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type TodoStatus string

const (
	TodoStatusPending TodoStatus = "pending"
	TodoStatusDone    TodoStatus = "done"
)

type Todo struct {
	Description string     `yaml:"description"`
	Status      TodoStatus `yaml:"status"`
	Worktree    string     `yaml:"worktree,omitempty"`
}

type TmuxWindow struct {
	Name    string  `yaml:"name"`
	Command *string `yaml:"command"`
}

type StorageBackend struct {
	Type          string `yaml:"type"` // "local" or "github"
	Owner         string `yaml:"owner,omitempty"`
	Repo          string `yaml:"repo,omitempty"`
	ProjectNumber int    `yaml:"project_number,omitempty"`
}

type Config struct {
	Name            string          `yaml:"name"`
	WorktreeNaming  string          `yaml:"worktree_naming"`
	StorageBackend  *StorageBackend `yaml:"storage_backend,omitempty"`
	Todos           []Todo          `yaml:"todos"`
	Windows         []TmuxWindow    `yaml:"windows"`
	configPath      string
}

const configFileName = "lfg-config.yaml"

// Load loads the config from the repository root, or creates a default one
func Load() (*Config, error) {
	repoRoot, err := getRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get repo root: %w", err)
	}

	configPath := filepath.Join(repoRoot, configFileName)

	// If config doesn't exist, run init wizard
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return runInitWizard(configPath, repoRoot)
	}

	// Load existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg.configPath = configPath
	return &cfg, nil
}

// Save saves the config to disk
func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(c.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AddTodo adds a new todo to the config
func (c *Config) AddTodo(description, worktree string) {
	// Add to the beginning of the list
	c.Todos = append([]Todo{{
		Description: description,
		Status:      TodoStatusPending,
		Worktree:    worktree,
	}}, c.Todos...)
}

// MarkTodoDone marks a todo as done by worktree name
func (c *Config) MarkTodoDone(worktree string) {
	for i := range c.Todos {
		if c.Todos[i].Worktree == worktree {
			c.Todos[i].Status = TodoStatusDone
			break
		}
	}
}

// GetTodoForWorktree returns the todo associated with a worktree
func (c *Config) GetTodoForWorktree(worktree string) *Todo {
	for i := range c.Todos {
		if c.Todos[i].Worktree == worktree {
			return &c.Todos[i]
		}
	}
	return nil
}

func getRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(output)), nil
}
