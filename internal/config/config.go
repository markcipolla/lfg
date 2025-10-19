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
	GitHubBody  string     `yaml:"github_body,omitempty"`
	GitHubURL   string     `yaml:"github_url,omitempty"`
}

type TmuxWindow struct {
	Name    string  `yaml:"name"`
	Command *string `yaml:"command"`
}

type Pane struct {
	Name    string  `yaml:"name"`
	Width   string  `yaml:"width,omitempty"`   // e.g. "50%", "33%"
	Command *string `yaml:"command,omitempty"`
}

type LayoutRow struct {
	Height  string  `yaml:"height"`            // Height as percentage of work area (the 90% below description)
	Name    string  `yaml:"name,omitempty"`    // For single-pane rows
	Command *string `yaml:"command,omitempty"` // For single-pane rows
	Panes   []Pane  `yaml:"panes,omitempty"`   // For multi-pane rows (split horizontally)
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
	Windows         []TmuxWindow    `yaml:"windows,omitempty"` // Deprecated, use Layout
	Layout          []LayoutRow     `yaml:"layout,omitempty"`
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

	return LoadFromPath(configPath)
}

// LoadFromPath loads the config from a specific path without running init wizard
func LoadFromPath(configPath string) (*Config, error) {
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

// GetConfigPath returns the path to the config file
func (c *Config) GetConfigPath() string {
	return c.configPath
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

// GetLayout returns the layout, converting from old Windows format if necessary
// Note: Description pane is automatic (always top 10%), so this only returns the work panes
func (c *Config) GetLayout() []LayoutRow {
	// If we have the new layout format, use it
	if len(c.Layout) > 0 {
		return c.Layout
	}

	// Convert old Windows format to Layout
	// The old format should be converted to the new format (excluding description)
	if len(c.Windows) > 0 {
		// Calculate equal height for all panes (as percentage of the 90% work space)
		height := fmt.Sprintf("%d%%", 100/len(c.Windows))

		layout := []LayoutRow{}
		// Convert each window to a row
		for _, w := range c.Windows {
			layout = append(layout, LayoutRow{
				Height:  height,
				Name:    w.Name,
				Command: w.Command,
			})
		}

		return layout
	}

	// No layout defined, return empty
	return nil
}

func getRepoRoot() (string, error) {
	// Try to get the main worktree root by listing all worktrees
	// The first worktree in the list is always the main worktree
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err == nil {
		// Parse the output to get the first worktree path
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "worktree ") {
				return strings.TrimPrefix(line, "worktree "), nil
			}
		}
	}

	// Fallback to rev-parse if worktree list fails (e.g., not using worktrees)
	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(output)), nil
}
