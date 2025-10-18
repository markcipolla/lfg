package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/markcipolla/lfg/internal/config"
	"github.com/markcipolla/lfg/internal/git"
	"github.com/markcipolla/lfg/internal/tmux"
)

type model struct {
	config         *config.Config
	worktrees      []git.Worktree
	list           list.Model
	creating       bool
	deleting       bool
	textInput      textinput.Model
	err            error
	width          int
	height         int
	selectedWorktree string
}

type worktreeItem struct {
	worktree git.Worktree
	todo     *config.Todo
}

func (i worktreeItem) Title() string {
	name := git.GetWorktreeName(i.worktree.Path)
	if i.todo != nil {
		status := "○"
		if i.todo.Status == config.TodoStatusDone {
			status = "✓"
		}
		return fmt.Sprintf("%s %s - %s", status, name, i.todo.Description)
	}
	return name
}

func (i worktreeItem) Description() string {
	if i.worktree.Branch != "" {
		return fmt.Sprintf("Branch: %s", strings.TrimPrefix(i.worktree.Branch, "refs/heads/"))
	}
	return i.worktree.Path
}

func (i worktreeItem) FilterValue() string {
	return git.GetWorktreeName(i.worktree.Path)
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

func Run(cfg *config.Config) error {
	// Check tmux
	if !tmux.IsInstalled() {
		return fmt.Errorf("tmux is not installed")
	}

	// Get worktrees
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return err
	}

	// Create list items
	items := make([]list.Item, 0, len(worktrees))
	for _, wt := range worktrees {
		name := git.GetWorktreeName(wt.Path)
		todo := cfg.GetTodoForWorktree(name)
		items = append(items, worktreeItem{worktree: wt, todo: todo})
	}

	// Create list
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(items, delegate, 0, 0)
	l.Title = "Git Worktrees"
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("n", "c"),
				key.WithHelp("n/c", "new"),
			),
			key.NewBinding(
				key.WithKeys("d"),
				key.WithHelp("d", "delete"),
			),
			key.NewBinding(
				key.WithKeys("r"),
				key.WithHelp("r", "refresh"),
			),
		}
	}

	// Create text input for new worktree
	ti := textinput.New()
	ti.Placeholder = cfg.WorktreeNaming
	ti.CharLimit = 100
	ti.Width = 50

	m := &model{
		config:    cfg,
		worktrees: worktrees,
		list:      l,
		textInput: ti,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	// Check if user selected a worktree
	result := finalModel.(*model)
	if result.selectedWorktree != "" {
		return git.JumpToWorktree(result.selectedWorktree, cfg)
	}

	return nil
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle text input mode
		if m.creating {
			switch msg.String() {
			case "enter":
				return m.handleCreateWorktree()
			case "esc":
				m.creating = false
				m.textInput.SetValue("")
				return m, nil
			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		// Handle delete confirmation
		if m.deleting {
			switch msg.String() {
			case "y", "Y":
				return m.handleDeleteWorktree()
			case "n", "N", "esc":
				m.deleting = false
				return m, nil
			}
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			if item, ok := m.list.SelectedItem().(worktreeItem); ok {
				m.selectedWorktree = git.GetWorktreeName(item.worktree.Path)
				return m, tea.Quit
			}

		case "n", "c":
			m.creating = true
			m.textInput.SetValue(m.config.WorktreeNaming)
			m.textInput.Focus()
			m.textInput.CursorEnd()
			return m, nil

		case "d":
			m.deleting = true
			return m, nil

		case "r":
			return m, m.refreshWorktrees
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)

	case refreshMsg:
		m.worktrees = msg.worktrees
		items := make([]list.Item, 0, len(m.worktrees))
		for _, wt := range m.worktrees {
			name := git.GetWorktreeName(wt.Path)
			todo := m.config.GetTodoForWorktree(name)
			items = append(items, worktreeItem{worktree: wt, todo: todo})
		}
		m.list.SetItems(items)
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}

	// Update list
	if !m.creating && !m.deleting {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) View() string {
	if m.creating {
		return m.viewCreateWorktree()
	}

	if m.deleting {
		return m.viewDeleteConfirm()
	}

	s := m.list.View()

	if m.err != nil {
		s += "\n\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	return s
}

func (m *model) viewCreateWorktree() string {
	// Show preview of what the worktree will be named
	preview := ""
	if m.textInput.Value() != "" {
		worktreeName := generateWorktreeName(m.config.Name, m.textInput.Value())
		preview = fmt.Sprintf("\nWorktree will be created as: %s",
			lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(worktreeName))
	}

	return fmt.Sprintf(
		"%s\n\nFeature Description:\n%s%s\n\n%s\n",
		titleStyle.Render("Create New Worktree"),
		m.textInput.View(),
		preview,
		helpStyle.Render("Enter: Create | Esc: Cancel"),
	)
}

func (m *model) viewDeleteConfirm() string {
	if item, ok := m.list.SelectedItem().(worktreeItem); ok {
		name := git.GetWorktreeName(item.worktree.Path)
		return fmt.Sprintf(
			"%s\n\nAre you sure you want to delete worktree '%s'?\n\n%s\n",
			titleStyle.Render("Delete Worktree"),
			name,
			helpStyle.Render("Y: Yes | N: No"),
		)
	}
	return ""
}

func (m *model) handleCreateWorktree() (tea.Model, tea.Cmd) {
	description := m.textInput.Value()
	if description == "" {
		m.err = fmt.Errorf("feature description cannot be empty")
		m.creating = false
		return m, nil
	}

	// Generate worktree name: [project-name]-[dasherized-description]
	worktreeName := generateWorktreeName(m.config.Name, description)

	// Create worktree
	if err := git.CreateWorktree(worktreeName); err != nil {
		m.err = err
		m.creating = false
		return m, nil
	}

	// Add todo with the original description
	m.config.AddTodo(description, worktreeName)
	if err := m.config.Save(); err != nil {
		m.err = fmt.Errorf("failed to save config: %w", err)
	}

	m.creating = false
	m.textInput.SetValue("")
	return m, m.refreshWorktrees
}

// generateWorktreeName creates a worktree name from project name and feature description
// Format: [project-name]-[dasherized-feature-name]
func generateWorktreeName(projectName, description string) string {
	// Dasherize the description
	dasherized := strings.ToLower(description)
	dasherized = strings.ReplaceAll(dasherized, " ", "-")
	// Remove special characters
	var result strings.Builder
	for _, r := range dasherized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	dasherized = result.String()

	// Remove consecutive dashes
	for strings.Contains(dasherized, "--") {
		dasherized = strings.ReplaceAll(dasherized, "--", "-")
	}

	// Trim dashes from start/end
	dasherized = strings.Trim(dasherized, "-")

	return projectName + "-" + dasherized
}

func (m *model) handleDeleteWorktree() (tea.Model, tea.Cmd) {
	if item, ok := m.list.SelectedItem().(worktreeItem); ok {
		name := git.GetWorktreeName(item.worktree.Path)

		// Delete worktree
		if err := git.DeleteWorktree(name, true); err != nil {
			m.err = err
			m.deleting = false
			return m, nil
		}

		// Mark todo as done
		m.config.MarkTodoDone(name)
		if err := m.config.Save(); err != nil {
			m.err = fmt.Errorf("failed to save config: %w", err)
		}

		m.deleting = false
		return m, m.refreshWorktrees
	}

	m.deleting = false
	return m, nil
}

type refreshMsg struct {
	worktrees []git.Worktree
}

type errMsg struct {
	err error
}

func (m *model) refreshWorktrees() tea.Msg {
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return errMsg{err: err}
	}
	return refreshMsg{worktrees: worktrees}
}
