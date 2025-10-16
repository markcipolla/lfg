use anyhow::{Context, Result};
use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode, KeyEventKind},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame, Terminal,
};
use std::io;

use crate::git::{self, Worktree};

enum InputMode {
    Normal,
    CreatingWorktree,
    Help,
    ConfirmDelete,
}

struct App {
    worktrees: Vec<Worktree>,
    list_state: ListState,
    input_mode: InputMode,
    input: String,
    branch_input: String,
    input_step: usize, // 0 = name, 1 = branch
    error_message: Option<String>,
    button_selected: bool, // true when "New Worktree" button is selected
    worktree_to_delete: Option<Worktree>,
    delete_is_dirty: bool,
}

impl App {
    fn new(initial_worktree: Option<String>) -> Result<Self> {
        let worktrees = git::list_worktrees().context("Failed to list worktrees")?;
        let mut list_state = ListState::default();

        // Select initial worktree (current worktree if provided, otherwise first one)
        if !worktrees.is_empty() {
            let initial_index = if let Some(name) = initial_worktree {
                worktrees.iter().position(|wt| wt.name == name).unwrap_or(0)
            } else {
                0
            };
            list_state.select(Some(initial_index));
        }

        Ok(Self {
            worktrees,
            list_state,
            input_mode: InputMode::Normal,
            input: String::new(),
            branch_input: String::new(),
            input_step: 0,
            error_message: None,
            button_selected: false,
            worktree_to_delete: None,
            delete_is_dirty: false,
        })
    }

    fn next(&mut self) {
        if self.button_selected {
            // From button, go to first item or stay on button if empty
            if !self.worktrees.is_empty() {
                self.button_selected = false;
                self.list_state.select(Some(0));
            }
        } else {
            // In list, navigate down or move to button
            let i = match self.list_state.selected() {
                Some(i) => {
                    if i >= self.worktrees.len() - 1 {
                        // Last item, move to button
                        self.button_selected = true;
                        self.list_state.select(None);
                        return;
                    } else {
                        i + 1
                    }
                }
                None => {
                    if self.worktrees.is_empty() {
                        self.button_selected = true;
                        return;
                    }
                    0
                }
            };
            self.list_state.select(Some(i));
        }
    }

    fn previous(&mut self) {
        if self.button_selected {
            // From button, go to last item
            if !self.worktrees.is_empty() {
                self.button_selected = false;
                self.list_state.select(Some(self.worktrees.len() - 1));
            }
        } else {
            // In list, navigate up or move to button
            let i = match self.list_state.selected() {
                Some(i) => {
                    if i == 0 {
                        // First item, move to button
                        self.button_selected = true;
                        self.list_state.select(None);
                        return;
                    } else {
                        i - 1
                    }
                }
                None => {
                    if self.worktrees.is_empty() {
                        self.button_selected = true;
                        return;
                    }
                    0
                }
            };
            self.list_state.select(Some(i));
        }
    }

    fn toggle_button_focus(&mut self) {
        if self.button_selected {
            // Move focus to list
            if !self.worktrees.is_empty() {
                self.button_selected = false;
                if self.list_state.selected().is_none() {
                    self.list_state.select(Some(0));
                }
            }
        } else {
            // Move focus to button
            self.button_selected = true;
            self.list_state.select(None);
        }
    }

    fn refresh_worktrees(&mut self) -> Result<()> {
        self.worktrees = git::list_worktrees()?;
        if !self.worktrees.is_empty() && self.list_state.selected().is_none() && !self.button_selected {
            self.list_state.select(Some(0));
        }
        Ok(())
    }

    fn start_create_worktree(&mut self) {
        self.input_mode = InputMode::CreatingWorktree;
        self.input.clear();
        self.branch_input.clear();
        self.input_step = 0;
        self.error_message = None;
    }

    fn toggle_help(&mut self) {
        self.input_mode = match self.input_mode {
            InputMode::Help => InputMode::Normal,
            _ => InputMode::Help,
        };
    }

    fn start_delete_worktree(&mut self) -> Result<()> {
        if let Some(i) = self.list_state.selected() {
            if i < self.worktrees.len() {
                let worktree = self.worktrees[i].clone();

                // Check if worktree has uncommitted changes
                let is_dirty = git::is_worktree_dirty(&worktree.path)?;

                self.worktree_to_delete = Some(worktree);
                self.delete_is_dirty = is_dirty;
                self.input_mode = InputMode::ConfirmDelete;
            }
        }
        Ok(())
    }

    fn confirm_delete(&mut self) -> Result<()> {
        if let Some(worktree) = &self.worktree_to_delete {
            let force = self.delete_is_dirty;

            // Check if we're in a tmux session with the same name as the worktree
            let current_session = crate::tmux::get_current_session();
            let should_kill_session = current_session.as_ref() == Some(&worktree.name);

            match git::delete_worktree(&worktree.path, force) {
                Ok(_) => {
                    // If we were in the tmux session for this worktree, kill it
                    if should_kill_session {
                        if let Err(e) = crate::tmux::kill_session(&worktree.name) {
                            eprintln!("Warning: Failed to kill tmux session: {}", e);
                        }
                    }

                    self.refresh_worktrees()?;
                    self.cancel_delete();
                }
                Err(e) => {
                    self.error_message = Some(format!("Failed to delete worktree: {}", e));
                    self.cancel_delete();
                }
            }
        }
        Ok(())
    }

    fn cancel_delete(&mut self) {
        self.input_mode = InputMode::Normal;
        self.worktree_to_delete = None;
        self.delete_is_dirty = false;
    }

    fn cancel_input(&mut self) {
        self.input_mode = InputMode::Normal;
        self.input.clear();
        self.branch_input.clear();
        self.input_step = 0;
        self.error_message = None;
    }

    fn submit_input(&mut self) -> Result<()> {
        match self.input_step {
            0 => {
                // Move to branch input
                self.input_step = 1;
            }
            1 => {
                // Create worktree
                let name = self.input.clone();
                let branch = if self.branch_input.is_empty() {
                    None
                } else {
                    Some(self.branch_input.as_str())
                };

                match git::create_worktree(&name, branch) {
                    Ok(_) => {
                        self.refresh_worktrees()?;
                        self.cancel_input();
                        // Select the newly created worktree
                        if let Some(pos) = self.worktrees.iter().position(|wt| wt.name == name) {
                            self.list_state.select(Some(pos));
                        }
                    }
                    Err(e) => {
                        self.error_message = Some(format!("Failed to create worktree: {}", e));
                    }
                }
            }
            _ => {}
        }
        Ok(())
    }

    fn current_input_mut(&mut self) -> &mut String {
        match self.input_step {
            0 => &mut self.input,
            1 => &mut self.branch_input,
            _ => &mut self.input,
        }
    }
}

pub fn run() -> Result<()> {
    // Setup terminal
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    // Detect current worktree to highlight it
    let current_worktree = git::get_current_worktree().ok().flatten();

    // Create app and run
    let mut app = App::new(current_worktree)?;
    let res = run_app(&mut terminal, &mut app);

    // Restore terminal
    disable_raw_mode()?;
    execute!(
        terminal.backend_mut(),
        LeaveAlternateScreen,
        DisableMouseCapture
    )?;
    terminal.show_cursor()?;

    if let Err(err) = res {
        eprintln!("Error: {:?}", err);
    }

    Ok(())
}

fn run_app<B: ratatui::backend::Backend>(
    terminal: &mut Terminal<B>,
    app: &mut App,
) -> Result<()> {
    loop {
        terminal.draw(|f| ui(f, app))?;

        if let Event::Key(key) = event::read()? {
            if key.kind != KeyEventKind::Press {
                continue;
            }

            match app.input_mode {
                InputMode::Normal => match key.code {
                    KeyCode::Char('q') | KeyCode::Esc => return Ok(()),
                    KeyCode::Char('j') | KeyCode::Down => app.next(),
                    KeyCode::Char('k') | KeyCode::Up => app.previous(),
                    KeyCode::Tab => app.toggle_button_focus(),
                    KeyCode::Char('?') => app.toggle_help(),
                    KeyCode::Char('n') | KeyCode::Char('c') => app.start_create_worktree(),
                    KeyCode::Char('d') | KeyCode::Delete => {
                        app.start_delete_worktree()?;
                    }
                    KeyCode::Char('r') => {
                        app.refresh_worktrees()?;
                    }
                    KeyCode::Enter => {
                        if app.button_selected {
                            // Button selected, create new worktree
                            app.start_create_worktree();
                        } else if let Some(i) = app.list_state.selected() {
                            if i < app.worktrees.len() {
                                let worktree = &app.worktrees[i];
                                // Exit TUI and start tmux session
                                disable_raw_mode()?;
                                return crate::tmux::start_session(&worktree.name, &worktree.path);
                            }
                        }
                    }
                    _ => {}
                },
                InputMode::Help => match key.code {
                    KeyCode::Char('q') | KeyCode::Esc | KeyCode::Char('?') => app.toggle_help(),
                    _ => {}
                },
                InputMode::ConfirmDelete => match key.code {
                    KeyCode::Char('y') | KeyCode::Char('Y') | KeyCode::Enter => {
                        app.confirm_delete()?;
                    }
                    KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                        app.cancel_delete();
                    }
                    _ => {}
                },
                InputMode::CreatingWorktree => match key.code {
                    KeyCode::Enter => {
                        app.submit_input()?;
                    }
                    KeyCode::Esc => {
                        app.cancel_input();
                    }
                    KeyCode::Char(c) => {
                        app.current_input_mut().push(c);
                    }
                    KeyCode::Backspace => {
                        app.current_input_mut().pop();
                    }
                    _ => {}
                },
            }
        }
    }
}

fn ui(f: &mut Frame, app: &App) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(0),
            Constraint::Length(3),
        ])
        .split(f.area());

    // Title
    let title = Paragraph::new("LFG - Git Worktree Manager")
        .style(Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD))
        .block(Block::default().borders(Borders::ALL));
    f.render_widget(title, chunks[0]);

    // Main content
    match app.input_mode {
        InputMode::Normal => {
            render_worktree_list(f, app, chunks[1]);
            render_help(f, chunks[2]);
        }
        InputMode::CreatingWorktree => {
            render_create_worktree(f, app, chunks[1]);
            render_input_help(f, chunks[2]);
        }
        InputMode::Help => {
            render_full_help(f, chunks[1]);
            let help_footer = Paragraph::new("Press ? or Esc to close")
                .style(Style::default().fg(Color::Gray))
                .block(Block::default().borders(Borders::ALL));
            f.render_widget(help_footer, chunks[2]);
        }
        InputMode::ConfirmDelete => {
            render_worktree_list(f, app, chunks[1]);
            render_confirm_delete(f, app, chunks[2]);
        }
    }
}

fn render_worktree_list(f: &mut Frame, app: &App, area: Rect) {
    // Split area into list and button sections
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Min(0), Constraint::Length(3)])
        .split(area);

    // Render worktree list
    let items: Vec<ListItem> = app
        .worktrees
        .iter()
        .map(|wt| {
            let content = vec![Line::from(vec![
                Span::styled(
                    format!("{:<20}", wt.name),
                    Style::default().fg(Color::Green),
                ),
                Span::styled(
                    format!(" {} ", wt.branch),
                    Style::default().fg(Color::Yellow),
                ),
                Span::styled(
                    wt.path.display().to_string(),
                    Style::default().fg(Color::Gray),
                ),
            ])];
            ListItem::new(content)
        })
        .collect();

    let list = List::new(items)
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title("Worktrees (↑↓/jk to navigate, Tab to toggle, Enter to select)"),
        )
        .highlight_style(
            Style::default()
                .bg(Color::DarkGray)
                .add_modifier(Modifier::BOLD),
        )
        .highlight_symbol(">> ");

    f.render_stateful_widget(list, chunks[0], &mut app.list_state.clone());

    // Render "New Worktree" button
    let button_style = if app.button_selected {
        Style::default()
            .fg(Color::Black)
            .bg(Color::Cyan)
            .add_modifier(Modifier::BOLD)
    } else {
        Style::default()
            .fg(Color::Cyan)
            .add_modifier(Modifier::BOLD)
    };

    let button_text = if app.button_selected {
        "[ ✨ New Worktree ]"
    } else {
        "  ✨ New Worktree  "
    };

    let button = Paragraph::new(button_text)
        .style(button_style)
        .block(Block::default().borders(Borders::ALL));

    f.render_widget(button, chunks[1]);
}

fn render_create_worktree(f: &mut Frame, app: &App, area: Rect) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(3), Constraint::Length(3), Constraint::Min(0)])
        .split(area);

    let name_style = if app.input_step == 0 {
        Style::default().fg(Color::Yellow)
    } else {
        Style::default()
    };

    let branch_style = if app.input_step == 1 {
        Style::default().fg(Color::Yellow)
    } else {
        Style::default()
    };

    let name_input = Paragraph::new(app.input.as_str())
        .style(name_style)
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title("Worktree Name"),
        );
    f.render_widget(name_input, chunks[0]);

    let branch_input = Paragraph::new(app.branch_input.as_str())
        .style(branch_style)
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title("Branch Name (optional)"),
        );
    f.render_widget(branch_input, chunks[1]);

    if let Some(error) = &app.error_message {
        let error_widget = Paragraph::new(error.as_str())
            .style(Style::default().fg(Color::Red))
            .block(Block::default().borders(Borders::ALL).title("Error"));
        f.render_widget(error_widget, chunks[2]);
    }
}

fn render_help(f: &mut Frame, area: Rect) {
    let width = area.width;

    // Choose help text based on available width
    let help_text = if width >= 90 {
        // Full help text for wide screens
        "q: Quit | n: New | d: Delete | r: Refresh | Tab: Toggle | Enter: Select | ?: Help"
    } else if width >= 70 {
        // Medium screens - abbreviate slightly
        "q: Quit | n: New | d: Delete | r: Refresh | Tab: Toggle | ?: Help"
    } else if width >= 50 {
        // Small screens - more compact
        "q: Quit | n: New | d: Del | r: Refresh | ?: Help"
    } else {
        // Very small screens - minimal
        "q: Quit | n: New | d: Del | ?: Help"
    };

    let help = Paragraph::new(help_text)
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL).title("Keys"));
    f.render_widget(help, area);
}

fn render_input_help(f: &mut Frame, area: Rect) {
    let help = Paragraph::new("Enter: Next/Create | Esc: Cancel")
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL));
    f.render_widget(help, area);
}

fn render_full_help(f: &mut Frame, area: Rect) {
    let help_text = vec![
        Line::from(vec![
            Span::styled("Navigation", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
        ]),
        Line::from(""),
        Line::from(vec![
            Span::styled("  ↑/k        ", Style::default().fg(Color::Yellow)),
            Span::raw("Move selection up"),
        ]),
        Line::from(vec![
            Span::styled("  ↓/j        ", Style::default().fg(Color::Yellow)),
            Span::raw("Move selection down"),
        ]),
        Line::from(vec![
            Span::styled("  Tab        ", Style::default().fg(Color::Yellow)),
            Span::raw("Toggle between list and New button"),
        ]),
        Line::from(vec![
            Span::styled("  Enter      ", Style::default().fg(Color::Yellow)),
            Span::raw("Select worktree or activate button"),
        ]),
        Line::from(""),
        Line::from(vec![
            Span::styled("Actions", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
        ]),
        Line::from(""),
        Line::from(vec![
            Span::styled("  n/c        ", Style::default().fg(Color::Yellow)),
            Span::raw("Create new worktree"),
        ]),
        Line::from(vec![
            Span::styled("  d          ", Style::default().fg(Color::Yellow)),
            Span::raw("Delete selected worktree"),
        ]),
        Line::from(vec![
            Span::styled("  r          ", Style::default().fg(Color::Yellow)),
            Span::raw("Refresh worktree list"),
        ]),
        Line::from(vec![
            Span::styled("  ?          ", Style::default().fg(Color::Yellow)),
            Span::raw("Toggle this help screen"),
        ]),
        Line::from(vec![
            Span::styled("  q/Esc      ", Style::default().fg(Color::Yellow)),
            Span::raw("Quit application"),
        ]),
    ];

    let help = Paragraph::new(help_text)
        .block(Block::default().borders(Borders::ALL).title("Help"))
        .style(Style::default().fg(Color::Gray));

    f.render_widget(help, area);
}

fn render_confirm_delete(f: &mut Frame, app: &App, area: Rect) {
    if let Some(worktree) = &app.worktree_to_delete {
        let message = if app.delete_is_dirty {
            vec![
                Line::from(vec![
                    Span::styled("⚠ WARNING: ", Style::default().fg(Color::Red).add_modifier(Modifier::BOLD)),
                    Span::styled("This worktree has uncommitted changes!", Style::default().fg(Color::Yellow)),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::raw("Delete worktree '"),
                    Span::styled(&worktree.name, Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
                    Span::raw("'?"),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("Y", Style::default().fg(Color::Green).add_modifier(Modifier::BOLD)),
                    Span::raw("es (force delete) | "),
                    Span::styled("N", Style::default().fg(Color::Red).add_modifier(Modifier::BOLD)),
                    Span::raw("o / Esc"),
                ]),
            ]
        } else {
            vec![
                Line::from(vec![
                    Span::raw("Delete worktree '"),
                    Span::styled(&worktree.name, Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
                    Span::raw("'?"),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("Y", Style::default().fg(Color::Green).add_modifier(Modifier::BOLD)),
                    Span::raw("es | "),
                    Span::styled("N", Style::default().fg(Color::Red).add_modifier(Modifier::BOLD)),
                    Span::raw("o / Esc"),
                ]),
            ]
        };

        let confirm = Paragraph::new(message)
            .block(Block::default().borders(Borders::ALL).title("Confirm Delete"))
            .style(Style::default());

        f.render_widget(confirm, area);
    }
}
