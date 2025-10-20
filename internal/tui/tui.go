package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/markcipolla/lfg/internal/config"
	"github.com/markcipolla/lfg/internal/git"
	"github.com/markcipolla/lfg/internal/github"
	"github.com/markcipolla/lfg/internal/tmux"
)

type model struct {
	config         *config.Config
	worktrees      []git.Worktree
	list           list.Model
	creating       bool
	deleting       bool
	textInput      textinput.Model
	spinner        spinner.Model
	loading        bool
	err            error
	width          int
	height         int
	selectedWorktree string
	exitToMain     bool // true if user selected main worktree to exit current session
}

type worktreeItem struct {
	worktree    git.Worktree
	todo        *config.Todo
	githubItem  *github.ProjectItem
	isCheckedOut bool // true if there's a worktree for this item
}

func (i worktreeItem) Title() string {
	// GitHub item without worktree
	if i.githubItem != nil && !i.isCheckedOut {
		status := "○"
		if i.githubItem.Status == "Done" {
			status = "✓"
		}
		return fmt.Sprintf("%s %s", status, i.githubItem.Title)
	}

	// Worktree with or without todo
	name := git.GetWorktreeName(i.worktree.Path)
	if i.todo != nil {
		status := "○"
		if i.todo.Status == config.TodoStatusDone {
			status = "✓"
		}
		return fmt.Sprintf("%s %s - %s", status, name, i.todo.Description)
	}
	if i.githubItem != nil {
		status := "●" // Checked out indicator
		if i.githubItem.Status == "Done" {
			status = "✓"
		}
		return fmt.Sprintf("%s %s - %s", status, name, i.githubItem.Title)
	}
	return name
}

func (i worktreeItem) Description() string {
	// GitHub item without worktree
	if i.githubItem != nil && !i.isCheckedOut {
		statusText := ""
		if i.githubItem.Status != "" {
			statusText = fmt.Sprintf("Status: %s", i.githubItem.Status)
		}
		if i.githubItem.Content.Number > 0 {
			return fmt.Sprintf("Issue #%d | %s", i.githubItem.Content.Number, statusText)
		}
		return statusText
	}

	// Worktree
	if i.worktree.Branch != "" {
		branch := strings.TrimPrefix(i.worktree.Branch, "refs/heads/")
		if i.githubItem != nil && i.githubItem.Status != "" {
			return fmt.Sprintf("Branch: %s | Status: %s", branch, i.githubItem.Status)
		}
		return fmt.Sprintf("Branch: %s", branch)
	}
	return i.worktree.Path
}

func (i worktreeItem) FilterValue() string {
	if i.githubItem != nil && !i.isCheckedOut {
		return i.githubItem.Title
	}
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

type Result struct {
	SelectedWorktree string
	ExitToMain       bool
}

func Run(cfg *config.Config) (*Result, error) {
	// Check tmux
	if !tmux.IsInstalled() {
		return nil, fmt.Errorf("tmux is not installed")
	}

	// Get current worktree if we're in one
	currentWorktree, err := git.GetCurrentWorktree()
	if err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: failed to detect current worktree: %v\n", err)
	}

	// Get worktrees
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return nil, err
	}

	// Create initial list items for worktrees (without GitHub data)
	items := make([]list.Item, 0, len(worktrees))
	currentWorktreeIndex := -1

	for _, wt := range worktrees {
		name := git.GetWorktreeName(wt.Path)
		todo := cfg.GetTodoForWorktree(name)

		// Check if this is the current worktree
		if currentWorktree != "" && name == currentWorktree {
			currentWorktreeIndex = len(items)
		}

		items = append(items, worktreeItem{
			worktree:    wt,
			todo:        todo,
			githubItem:  nil,
			isCheckedOut: true,
		})
	}

	// Create list
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(items, delegate, 80, 20) // Initial size, will be updated by WindowSizeMsg
	l.Title = "" // No title - we show it in our custom header
	l.SetShowTitle(false)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
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

	// Select the current worktree if found
	if currentWorktreeIndex >= 0 {
		l.Select(currentWorktreeIndex)
	}

	// Create text input for new worktree
	ti := textinput.New()
	ti.Placeholder = cfg.WorktreeNaming
	ti.CharLimit = 100
	ti.Width = 50

	// Create spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := &model{
		config:    cfg,
		worktrees: worktrees,
		list:      l,
		textInput: ti,
		spinner:   s,
		loading:   cfg.StorageBackend != nil && cfg.StorageBackend.Type == "github",
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	// Return the result
	result := finalModel.(*model)
	return &Result{
		SelectedWorktree: result.selectedWorktree,
		ExitToMain:       result.exitToMain,
	}, nil
}

func (m *model) Init() tea.Cmd {
	// Start spinner and fetch GitHub data if configured
	if m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
		return tea.Batch(m.spinner.Tick, m.fetchGithubItems)
	}
	return nil
}

type githubItemsMsg struct {
	items []github.ProjectItem
	err   error
}

func (m *model) fetchGithubItems() tea.Msg {
	if m.config.StorageBackend == nil || m.config.StorageBackend.Type != "github" {
		return githubItemsMsg{items: nil, err: nil}
	}

	items, err := github.ListProjectItems(
		m.config.StorageBackend.Owner,
		m.config.StorageBackend.Repo,
		m.config.StorageBackend.ProjectNumber,
	)
	return githubItemsMsg{items: items, err: err}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case githubItemsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = fmt.Errorf("failed to fetch GitHub items: %w", msg.err)
		} else if msg.items != nil {
			// Merge GitHub items with existing worktree items
			m.mergeGithubItems(msg.items)
		}
		return m, nil

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
				// If it's a GitHub item without a worktree, create one
				if item.githubItem != nil && !item.isCheckedOut {
					return m.handleCreateWorktreeFromGithub(item.githubItem)
				}

				// Check if this is the main worktree (first in the list)
				name := git.GetWorktreeName(item.worktree.Path)
				isMainWorktree := false
				if len(m.worktrees) > 0 {
					mainName := git.GetWorktreeName(m.worktrees[0].Path)
					isMainWorktree = (name == mainName)
				}

				// If it's the main worktree, set flag to exit current session
				if isMainWorktree {
					m.exitToMain = true
					m.selectedWorktree = name
					return m, tea.Quit
				}

				// Otherwise jump to the selected worktree
				m.selectedWorktree = name
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
			// Show spinner if GitHub is configured
			if m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.refreshAll)
			}
			return m, m.refreshWorktrees
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Account for header (2 lines) + potential error line (1 line)
		m.list.SetSize(msg.Width, msg.Height-3)

	case refreshMsg:
		m.worktrees = msg.worktrees
		// Just update worktrees list with current items (no GitHub fetch)
		items := make([]list.Item, 0, len(m.worktrees))
		for _, wt := range m.worktrees {
			name := git.GetWorktreeName(wt.Path)
			todo := m.config.GetTodoForWorktree(name)
			items = append(items, worktreeItem{
				worktree:    wt,
				todo:        todo,
				githubItem:  nil,
				isCheckedOut: true,
			})
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

	// Build the view with header
	var view strings.Builder

	// Show header
	header := titleStyle.Render("LFG - Git Worktrees")
	view.WriteString(header)
	view.WriteString("\n")

	// Show loading spinner if fetching GitHub data
	if m.loading {
		view.WriteString("\n")
		view.WriteString(m.spinner.View())
		view.WriteString(" Fetching GitHub project items...")
		return view.String()
	}

	view.WriteString("\n")

	// Show list
	view.WriteString(m.list.View())

	// Show error if present
	if m.err != nil {
		view.WriteString("\n")
		view.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return view.String()
}

func (m *model) mergeGithubItems(githubItems []github.ProjectItem) {
	// Create a map of worktree names for quick lookup
	worktreeMap := make(map[string]git.Worktree)
	for _, wt := range m.worktrees {
		name := git.GetWorktreeName(wt.Path)
		worktreeMap[name] = wt
	}

	// Track which GitHub items have been matched to worktrees
	matchedGithubItems := make(map[string]bool)

	// Create list items
	items := make([]list.Item, 0, len(m.worktrees)+len(githubItems))

	for _, wt := range m.worktrees {
		name := git.GetWorktreeName(wt.Path)
		todo := m.config.GetTodoForWorktree(name)

		// Try to match with GitHub item
		var matchedItem *github.ProjectItem
		for i := range githubItems {
			item := &githubItems[i]
			// Match by worktree name or issue number
			itemName := generateWorktreeName(m.config.Name, item.Title)
			if itemName == name || (item.Content.Number > 0 && fmt.Sprintf("issue-%d", item.Content.Number) == name) {
				matchedItem = item
				matchedGithubItems[item.ID] = true

				// Update the todo with GitHub data if it exists
				if todo != nil {
					// Get the body from the content if available
					if item.Content.Body != "" {
						todo.GitHubBody = item.Content.Body
					} else if item.Body != "" {
						todo.GitHubBody = item.Body
					}
					if item.Content.URL != "" {
						todo.GitHubURL = item.Content.URL
					}
					// Save the updated config
					m.config.Save()
				}

				// If this item has a worktree but isn't in "In Progress" or "Done", move it to "In Progress"
				if m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
					if item.Status != "In Progress" && item.Status != "Done" {
						err := github.UpdateProjectItemStatus(
							m.config.StorageBackend.Owner,
							m.config.StorageBackend.Repo,
							m.config.StorageBackend.ProjectNumber,
							item.ID,
							"In Progress",
						)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Warning: failed to update item status to In Progress: %v\n", err)
						} else {
							// Update the local copy
							item.Status = "In Progress"
						}
					}
				}

				break
			}
		}

		items = append(items, worktreeItem{
			worktree:    wt,
			todo:        todo,
			githubItem:  matchedItem,
			isCheckedOut: true,
		})
	}

	// Add GitHub items that don't have worktrees
	for i := range githubItems {
		item := &githubItems[i]
		if !matchedGithubItems[item.ID] {
			items = append(items, worktreeItem{
				githubItem:  item,
				isCheckedOut: false,
			})
		}
	}

	m.list.SetItems(items)
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

	// If GitHub is configured, show spinner and create item + refresh in background
	if m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
		m.loading = true
		return m, tea.Batch(
			m.spinner.Tick,
			m.createGithubItemAndRefresh(description, worktreeName),
		)
	}

	// Otherwise just refresh
	return m, m.refreshWorktrees
}

type createItemMsg struct {
	err error
}

func (m *model) createGithubItemAndRefresh(description, worktreeName string) tea.Cmd {
	return func() tea.Msg {
		// Create GitHub Project item
		item, err := github.CreateProjectItem(
			m.config.StorageBackend.Owner,
			m.config.StorageBackend.Repo,
			m.config.StorageBackend.ProjectNumber,
			description,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create GitHub project item: %v\n", err)
			return createItemMsg{err: err}
		}

		// Move to In Progress since we're creating a worktree
		err = github.UpdateProjectItemStatus(
			m.config.StorageBackend.Owner,
			m.config.StorageBackend.Repo,
			m.config.StorageBackend.ProjectNumber,
			item.ID,
			"In Progress",
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update item status: %v\n", err)
		}

		// Refresh to get all items
		return m.fetchGithubItems()
	}
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

func (m *model) handleCreateWorktreeFromGithub(item *github.ProjectItem) (tea.Model, tea.Cmd) {
	// Generate worktree name from the GitHub item title
	worktreeName := generateWorktreeName(m.config.Name, item.Title)

	// Create worktree
	if err := git.CreateWorktree(worktreeName); err != nil {
		m.err = err
		return m, nil
	}

	// Update GitHub item status to In Progress
	if m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
		err := github.UpdateProjectItemStatus(
			m.config.StorageBackend.Owner,
			m.config.StorageBackend.Repo,
			m.config.StorageBackend.ProjectNumber,
			item.ID,
			"In Progress",
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update item status: %v\n", err)
		}
	}

	// Add todo with the GitHub item title and body
	m.config.AddTodo(item.Title, worktreeName)
	todo := m.config.GetTodoForWorktree(worktreeName)
	if todo != nil {
		todo.GitHubBody = item.Content.Body
		todo.GitHubURL = item.Content.URL
	}
	if err := m.config.Save(); err != nil {
		m.err = fmt.Errorf("failed to save config: %w", err)
	}

	// Set as selected and quit to jump to it
	m.selectedWorktree = worktreeName
	return m, tea.Quit
}

func (m *model) handleDeleteWorktree() (tea.Model, tea.Cmd) {
	if item, ok := m.list.SelectedItem().(worktreeItem); ok {
		// Get the name from either the worktree or the todo
		var name string
		if item.isCheckedOut {
			name = git.GetWorktreeName(item.worktree.Path)
		} else if item.todo != nil {
			name = item.todo.Worktree
		} else if item.githubItem != nil {
			// GitHub item without worktree - nothing to delete from git
			// Just remove from GitHub project if needed
			if m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
				err := github.UpdateProjectItemStatus(
					m.config.StorageBackend.Owner,
					m.config.StorageBackend.Repo,
					m.config.StorageBackend.ProjectNumber,
					item.githubItem.ID,
					"Done",
				)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to update item status to Done: %v\n", err)
				}
			}
			m.deleting = false
			return m, m.refreshWorktrees
		} else {
			// No way to identify this item
			m.deleting = false
			return m, nil
		}

		// Check if branch is merged
		isMerged, err := git.IsBranchMerged(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to check if branch is merged: %v\n", err)
		}

		// Update GitHub item status to Done if merged
		if isMerged && item.githubItem != nil && m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
			err := github.UpdateProjectItemStatus(
				m.config.StorageBackend.Owner,
				m.config.StorageBackend.Repo,
				m.config.StorageBackend.ProjectNumber,
				item.githubItem.ID,
				"Done",
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to update item status to Done: %v\n", err)
			}
		}

		// Check if we're deleting the current worktree
		currentWorktree, err := git.GetCurrentWorktree()
		isDeletingCurrent := err == nil && currentWorktree == name

		// Kill tmux session if it exists
		sessionName := tmux.SanitizeSessionName(name)
		if tmux.SessionExists(sessionName) {
			if err := tmux.KillSession(sessionName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to kill tmux session: %v\n", err)
			}
		}

		// Delete worktree
		if err := git.DeleteWorktree(name, true); err != nil {
			m.err = err
			m.deleting = false
			return m, nil
		}

		// Remove todo entirely (don't just mark as done)
		m.config.RemoveTodo(name)
		if err := m.config.Save(); err != nil {
			m.err = fmt.Errorf("failed to save config: %w", err)
		}

		m.deleting = false

		// If we deleted the current worktree, exit the TUI
		// The user will be returned to their shell (in the main repo)
		if isDeletingCurrent {
			return m, tea.Quit
		}

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

func (m *model) refreshAll() tea.Msg {
	// First refresh worktrees
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return errMsg{err: err}
	}
	m.worktrees = worktrees

	// Then fetch GitHub items
	return m.fetchGithubItems()
}
