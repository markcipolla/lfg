package config

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/markcipolla/lfg/internal/github"
)

func runInitWizard(configPath, repoRoot string) (*Config, error) {
	// Get default project name from directory
	defaultName := filepath.Base(repoRoot)

	m := &initModel{
		step:        stepProjectName,
		projectName: defaultName,
		configPath:  configPath,
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run init wizard: %w", err)
	}

	result := finalModel.(*initModel)
	if result.cancelled {
		return nil, fmt.Errorf("initialization cancelled")
	}

	return result.config, nil
}

type initStep int

const (
	stepProjectName initStep = iota
	stepStorageBackend
	stepGitHubAuth
	stepGitHubProjectSelect
	stepGitHubProjectName
	stepComplete
)

type initModel struct {
	step            initStep
	projectName     string
	storageChoice   int // 0 = Local, 1 = GitHub
	githubSetup     *githubSetupState
	configPath      string
	config          *Config
	cancelled       bool
	width           int
	height          int
}

type githubSetupState struct {
	owner           string
	repo            string
	projects        []githubProject
	selectedProject int
	projectName     string
	authStatus      string
	authError       string
}

type githubProject struct {
	ID     string
	Number int
	Title  string
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

func (m *initModel) Init() tea.Cmd {
	return nil
}

func (m *initModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit

		case "enter":
			return m.handleEnter()

		case "up", "k":
			return m.handleUp()

		case "down", "j":
			return m.handleDown()

		case "backspace":
			return m.handleBackspace()

		case "a":
			if m.step == stepGitHubAuth {
				return m.handleGitHubAuth()
			}

		default:
			// Handle character input for text fields
			if len(msg.String()) == 1 {
				return m.handleChar(msg.String()[0])
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case authCheckMsg:
		m.githubSetup = msg.setup
		// Automatically proceed if auth was successful
		if msg.setup != nil && msg.setup.authError == "" {
			if len(msg.setup.projects) > 0 {
				m.step = stepGitHubProjectSelect
			} else {
				m.step = stepGitHubProjectName
			}
		}
		return m, nil

	case projectCreateMsg:
		if msg.err != nil {
			if m.githubSetup != nil {
				m.githubSetup.authError = fmt.Sprintf("Failed to create project: %v", msg.err)
			}
			m.step = stepGitHubProjectName
			return m, nil
		}

		// Project created successfully
		backend := &StorageBackend{
			Type:          "github",
			Owner:         m.githubSetup.owner,
			Repo:          m.githubSetup.repo,
			ProjectNumber: msg.project.Number,
		}
		return m.completeSetup(backend)
	}

	return m, nil
}

func (m *initModel) View() string {
	switch m.step {
	case stepProjectName:
		return m.viewProjectName()
	case stepStorageBackend:
		return m.viewStorageBackend()
	case stepGitHubAuth:
		return m.viewGitHubAuth()
	case stepGitHubProjectSelect:
		return m.viewGitHubProjectSelect()
	case stepGitHubProjectName:
		return m.viewGitHubProjectName()
	case stepComplete:
		return m.viewComplete()
	}
	return ""
}

func (m *initModel) viewProjectName() string {
	return fmt.Sprintf(
		"%s\n\nProject Name:\n> %s\n\n%s\n",
		titleStyle.Render("LFG Initialization"),
		m.projectName,
		helpStyle.Render("Enter: Continue | Type to edit | Esc: Cancel"),
	)
}

func (m *initModel) viewStorageBackend() string {
	options := []string{
		"Local YAML (todos stored in lfg-config.yaml)",
		"GitHub Projects (todos synced with GitHub)",
	}

	result := titleStyle.Render("Choose Todo Storage Backend") + "\n\n"
	for i, opt := range options {
		cursor := "  "
		if i == m.storageChoice {
			cursor = "> "
			result += selectedStyle.Render(cursor + opt) + "\n"
		} else {
			result += cursor + opt + "\n"
		}
	}

	result += "\n" + helpStyle.Render("↑↓/jk: Navigate | Enter: Select | Esc: Cancel")
	return result
}

func (m *initModel) viewGitHubAuth() string {
	status := "Checking authentication..."
	if m.githubSetup != nil {
		if m.githubSetup.authError != "" {
			status = errorStyle.Render("✗ " + m.githubSetup.authError)
		} else if m.githubSetup.authStatus != "" {
			status = m.githubSetup.authStatus
		}
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n",
		titleStyle.Render("GitHub Authentication"),
		status,
		helpStyle.Render("a: Authenticate | Esc: Cancel"),
	)
}

func (m *initModel) viewGitHubProjectSelect() string {
	if m.githubSetup == nil || len(m.githubSetup.projects) == 0 {
		return "No projects found"
	}

	result := titleStyle.Render("Select GitHub Project") + "\n\n"
	for i, proj := range m.githubSetup.projects {
		cursor := "  "
		if i == m.githubSetup.selectedProject {
			cursor = "> "
			result += selectedStyle.Render(fmt.Sprintf("%s%s (Project #%d)", cursor, proj.Title, proj.Number)) + "\n"
		} else {
			result += fmt.Sprintf("%s%s (Project #%d)", cursor, proj.Title, proj.Number) + "\n"
		}
	}

	result += "\n" + helpStyle.Render("↑↓/jk: Navigate | Enter: Select | Esc: Cancel")
	return result
}

func (m *initModel) viewGitHubProjectName() string {
	repoInfo := ""
	if m.githubSetup != nil {
		repoInfo = fmt.Sprintf("%s/%s", m.githubSetup.owner, m.githubSetup.repo)
	}

	errorMsg := ""
	if m.githubSetup != nil && m.githubSetup.authError != "" {
		errorMsg = "\n\n" + errorStyle.Render("Error: "+m.githubSetup.authError)
	}

	projectName := m.projectName
	if m.githubSetup != nil && m.githubSetup.projectName != "" {
		projectName = m.githubSetup.projectName
	}

	return fmt.Sprintf(
		"%s\n\nNo GitHub Projects found for %s\n\nProject Name:\n> %s%s\n\n%s\n",
		titleStyle.Render("Create GitHub Project"),
		repoInfo,
		projectName,
		errorMsg,
		helpStyle.Render("Enter: Create Project | Type to edit | Esc: Cancel"),
	)
}

func (m *initModel) viewComplete() string {
	backendInfo := "Local YAML"
	if m.config != nil && m.config.StorageBackend != nil && m.config.StorageBackend.Type == "github" {
		backendInfo = fmt.Sprintf("GitHub Projects (%s/%s #%d)",
			m.config.StorageBackend.Owner,
			m.config.StorageBackend.Repo,
			m.config.StorageBackend.ProjectNumber)
	}

	return fmt.Sprintf(
		"%s\n\n✓ Configuration created successfully!\n\nProject: %s\nStorage: %s\n\n%s\n",
		titleStyle.Render("Setup Complete"),
		m.projectName,
		backendInfo,
		helpStyle.Render("Press Enter to continue..."),
	)
}

func (m *initModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProjectName:
		m.step = stepStorageBackend
	case stepStorageBackend:
		if m.storageChoice == 1 {
			m.step = stepGitHubAuth
			return m, m.checkGitHubAuth
		}
		// Local storage selected
		return m.completeSetup(nil)
	case stepGitHubAuth:
		// Move to project selection or creation
		if m.githubSetup != nil && len(m.githubSetup.projects) > 0 {
			m.step = stepGitHubProjectSelect
		} else if m.githubSetup != nil {
			m.step = stepGitHubProjectName
		}
	case stepGitHubProjectSelect:
		if m.githubSetup != nil && m.githubSetup.selectedProject < len(m.githubSetup.projects) {
			proj := m.githubSetup.projects[m.githubSetup.selectedProject]
			backend := &StorageBackend{
				Type:          "github",
				Owner:         m.githubSetup.owner,
				Repo:          m.githubSetup.repo,
				ProjectNumber: proj.Number,
			}
			return m.completeSetup(backend)
		}
	case stepGitHubProjectName:
		// Create new project
		return m, m.createGitHubProject
	case stepComplete:
		return m, tea.Quit
	}
	return m, nil
}

func (m *initModel) handleUp() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepStorageBackend:
		m.storageChoice = (m.storageChoice + 1) % 2
	case stepGitHubProjectSelect:
		if m.githubSetup != nil && m.githubSetup.selectedProject > 0 {
			m.githubSetup.selectedProject--
		}
	}
	return m, nil
}

func (m *initModel) handleDown() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepStorageBackend:
		m.storageChoice = (m.storageChoice + 1) % 2
	case stepGitHubProjectSelect:
		if m.githubSetup != nil && m.githubSetup.selectedProject < len(m.githubSetup.projects)-1 {
			m.githubSetup.selectedProject++
		}
	}
	return m, nil
}

func (m *initModel) handleBackspace() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProjectName:
		if len(m.projectName) > 0 {
			m.projectName = m.projectName[:len(m.projectName)-1]
		}
	case stepGitHubProjectName:
		if m.githubSetup != nil && len(m.githubSetup.projectName) > 0 {
			m.githubSetup.projectName = m.githubSetup.projectName[:len(m.githubSetup.projectName)-1]
		}
	}
	return m, nil
}

func (m *initModel) handleChar(c byte) (tea.Model, tea.Cmd) {
	switch m.step {
	case stepProjectName:
		m.projectName += string(c)
	case stepGitHubProjectName:
		if m.githubSetup != nil {
			if m.githubSetup.projectName == "" {
				m.githubSetup.projectName = m.projectName
			}
			m.githubSetup.projectName += string(c)
		}
	}
	return m, nil
}

func (m *initModel) handleGitHubAuth() (tea.Model, tea.Cmd) {
	// This would trigger the actual GitHub authentication
	// For now, we'll implement a simplified version
	return m, m.checkGitHubAuth
}

type authCheckMsg struct {
	setup *githubSetupState
	err   error
}

type projectCreateMsg struct {
	project *githubProject
	err     error
}

func (m *initModel) checkGitHubAuth() tea.Msg {
	setup := &githubSetupState{}

	// Check if authenticated
	if !github.IsAuthenticated() {
		setup.authError = "Not authenticated. Press 'a' to authenticate with GitHub CLI"
		return authCheckMsg{setup: setup}
	}

	// Check if has required scopes
	hasScopes, err := github.HasRequiredScopes()
	if err != nil {
		setup.authError = fmt.Sprintf("Failed to check scopes: %v", err)
		return authCheckMsg{err: err, setup: setup}
	}

	if !hasScopes {
		setup.authError = "Missing required scopes. Press 'a' to re-authenticate with project and repo scopes"
		return authCheckMsg{setup: setup}
	}

	// Get repo info
	repoInfo, err := github.GetRepoInfo()
	if err != nil {
		setup.authError = fmt.Sprintf("Failed to get repository info: %v", err)
		return authCheckMsg{err: err, setup: setup}
	}

	setup.owner = repoInfo.Owner
	setup.repo = repoInfo.Name
	setup.authStatus = "✓ Authenticated successfully! Loading projects..."

	// List projects
	projects, err := github.ListProjects(repoInfo.Owner, repoInfo.Name)
	if err != nil {
		setup.authError = fmt.Sprintf("Failed to list projects: %v", err)
		return authCheckMsg{err: err, setup: setup}
	}

	// Convert to our type
	for _, p := range projects {
		setup.projects = append(setup.projects, githubProject{
			ID:     p.ID,
			Number: p.Number,
			Title:  p.Title,
		})
	}

	if len(setup.projects) > 0 {
		setup.authStatus = fmt.Sprintf("✓ Found %d project(s)", len(setup.projects))
	} else {
		setup.authStatus = "✓ Authenticated. No projects found, will create new one."
	}

	return authCheckMsg{setup: setup}
}

func (m *initModel) createGitHubProject() tea.Msg {
	if m.githubSetup == nil {
		return projectCreateMsg{err: fmt.Errorf("GitHub not set up")}
	}

	projectName := m.githubSetup.projectName
	if projectName == "" {
		projectName = m.projectName
	}

	project, err := github.CreateProject(m.githubSetup.owner, m.githubSetup.repo, projectName)
	if err != nil {
		return projectCreateMsg{err: err}
	}

	return projectCreateMsg{
		project: &githubProject{
			ID:     project.ID,
			Number: project.Number,
			Title:  project.Title,
		},
	}
}

func (m *initModel) completeSetup(backend *StorageBackend) (tea.Model, tea.Cmd) {
	// Create default config with new layout format
	// Description pane is automatic (always top 10%), so layout only defines the remaining 90%
	m.config = &Config{
		Name:           m.projectName,
		WorktreeNaming: "Add feature",
		StorageBackend: backend,
		Todos:          []Todo{},
		Layout: []LayoutRow{
			{
				Height: "33%",
				Name:   "code",
			},
			{
				Height: "34%",
				Name:   "server",
				Command: stringPtr("claude --dangerously-skip-permissions"),
			},
			{
				Height: "33%",
				Name:   "shell",
			},
		},
		configPath: m.configPath,
	}

	// Save config
	if err := m.config.Save(); err != nil {
		m.githubSetup = &githubSetupState{
			authError: fmt.Sprintf("Failed to save config: %v", err),
		}
		return m, nil
	}

	m.step = stepComplete
	return m, nil
}

func stringPtr(s string) *string {
	return &s
}
