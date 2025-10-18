package viewer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/markcipolla/lfg/internal/config"
)

type model struct {
	viewport viewport.Model
	content  string
	ready    bool
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func Run(worktreeName string, cfg *config.Config) error {
	// Find the todo for this worktree
	todo := cfg.GetTodoForWorktree(worktreeName)

	// Build markdown content
	var content strings.Builder
	content.WriteString("# 📋 " + worktreeName + "\n\n")

	if todo != nil {
		content.WriteString("## " + todo.Description + "\n\n")

		// Show GitHub body if available
		if todo.GitHubBody != "" {
			content.WriteString(todo.GitHubBody + "\n\n")
		}

		content.WriteString("**Status:** `" + string(todo.Status) + "`\n\n")

		// Add GitHub info if available
		if cfg.StorageBackend != nil && cfg.StorageBackend.Type == "github" {
			content.WriteString("---\n\n")
			content.WriteString("### GitHub Project\n\n")
			content.WriteString(cfg.StorageBackend.Owner + "/" +
				cfg.StorageBackend.Repo +
				" #" + fmt.Sprintf("%d", cfg.StorageBackend.ProjectNumber) + "\n\n")

			if todo.GitHubURL != "" {
				content.WriteString("**Issue:** " + todo.GitHubURL + "\n\n")
			}
		}
	} else {
		content.WriteString("_No description available._\n\n")
	}

	// Render markdown with glamour
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		return err
	}

	rendered, err := renderer.Render(content.String())
	if err != nil {
		return err
	}

	m := model{
		content: rendered,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-2)
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 2
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	help := helpStyle.Render("↑/↓: scroll • q: close")
	return fmt.Sprintf("%s\n%s", m.viewport.View(), help)
}
