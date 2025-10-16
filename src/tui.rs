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

use crate::config::AppConfig;
use crate::git::{self, Worktree};

enum InputMode {
    Normal,
    CreatingWorktree,
    Help,
    ConfirmDelete,
}

struct App {
    app_config: AppConfig,
    worktrees: Vec<Worktree>,
    list_state: ListState,
    input_mode: InputMode,
    todo_input: String,
    worktree_input: String,
    input_step: usize, // 0 = todo description, 1 = worktree name
    error_message: Option<String>,
    list_area: Rect,
    button_area: Rect,
    button_selected: bool, // true when "New Worktree" button is selected
    worktree_to_delete: Option<Worktree>,
    delete_is_dirty: bool,
}

impl App {
    fn new(initial_worktree: Option<String>) -> Result<Self> {
        let app_config = AppConfig::load().context("Failed to load config")?;
        let worktrees = git::list_worktrees().context("Failed to list worktrees")?;
        let mut list_state = ListState::default();

        // Select initial worktree (current worktree if provided, otherwise first one based on todos)
        if !app_config.todos.is_empty() {
            let initial_index = if let Some(name) = initial_worktree {
                // Find the todo linked to this worktree
                app_config
                    .todos
                    .iter()
                    .position(|todo| todo.worktree.as_ref().map(|w| w == &name).unwrap_or(false))
                    .unwrap_or(0)
            } else {
                0
            };
            list_state.select(Some(initial_index));
        }

        Ok(Self {
            app_config,
            worktrees,
            list_state,
            input_mode: InputMode::Normal,
            todo_input: String::new(),
            worktree_input: String::new(),
            input_step: 0,
            error_message: None,
            list_area: Rect::default(),
            button_area: Rect::default(),
            button_selected: false,
            worktree_to_delete: None,
            delete_is_dirty: false,
        })
    }

    fn handle_click(&self, x: u16, y: u16) -> Option<usize> {
        // Check if click is within list area
        if x >= self.list_area.x
            && x < self.list_area.x + self.list_area.width
            && y >= self.list_area.y
            && y < self.list_area.y + self.list_area.height
        {
            // Calculate which item was clicked (accounting for border)
            let relative_y = y.saturating_sub(self.list_area.y + 1); // +1 for top border
            let index = relative_y as usize;
            if index < self.app_config.todos.len() {
                return Some(index);
            }
        }
        None
    }

    fn is_new_button_clicked(&self, x: u16, y: u16) -> bool {
        x >= self.button_area.x
            && x < self.button_area.x + self.button_area.width
            && y >= self.button_area.y
            && y < self.button_area.y + self.button_area.height
    }

    fn dasherize(input: &str) -> String {
        input
            .to_lowercase()
            .split_whitespace()
            .collect::<Vec<&str>>()
            .join("-")
    }

    fn next(&mut self) {
        if self.button_selected {
            // From button, go to first item or stay on button if empty
            if !self.app_config.todos.is_empty() {
                self.button_selected = false;
                self.list_state.select(Some(0));
            }
        } else {
            // In list, navigate down or move to button
            let i = match self.list_state.selected() {
                Some(i) => {
                    if i >= self.app_config.todos.len() - 1 {
                        // Last item, move to button
                        self.button_selected = true;
                        self.list_state.select(None);
                        return;
                    } else {
                        i + 1
                    }
                }
                None => {
                    if self.app_config.todos.is_empty() {
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
            if !self.app_config.todos.is_empty() {
                self.button_selected = false;
                self.list_state
                    .select(Some(self.app_config.todos.len() - 1));
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
                    if self.app_config.todos.is_empty() {
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
            if !self.app_config.todos.is_empty() {
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
        // Also reload config to get updated todos
        self.app_config = AppConfig::load()?;
        if !self.app_config.todos.is_empty()
            && self.list_state.selected().is_none()
            && !self.button_selected
        {
            self.list_state.select(Some(0));
        }
        Ok(())
    }

    fn start_create_worktree(&mut self) {
        self.input_mode = InputMode::CreatingWorktree;
        // Start with empty inputs
        self.todo_input.clear();
        self.worktree_input.clear();
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
            if i < self.app_config.todos.len() {
                let todo = &self.app_config.todos[i];
                if let Some(ref worktree_name) = todo.worktree {
                    // Find the actual worktree
                    if let Some(worktree) = self
                        .worktrees
                        .iter()
                        .find(|wt| &wt.name == worktree_name)
                        .cloned()
                    {
                        // Check if worktree has uncommitted changes
                        let is_dirty = git::is_worktree_dirty(&worktree.path)?;

                        self.worktree_to_delete = Some(worktree);
                        self.delete_is_dirty = is_dirty;
                        self.input_mode = InputMode::ConfirmDelete;
                    }
                }
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
                    // Mark todo as done
                    self.app_config.mark_todo_done(&worktree.name);
                    self.app_config.save()?;

                    // If we were in the tmux session for this worktree, kill it
                    if should_kill_session {
                        if let Err(e) = crate::tmux::kill_session(&worktree.name) {
                            eprintln!("Warning: Failed to kill tmux session: {e}");
                        }
                    }

                    self.refresh_worktrees()?;
                    self.cancel_delete();
                }
                Err(e) => {
                    self.error_message = Some(format!("Failed to delete worktree: {e}"));
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

    fn can_create_worktree(&self) -> bool {
        !self.todo_input.trim().is_empty() && !self.worktree_input.trim().is_empty()
    }

    fn cancel_input(&mut self) {
        self.input_mode = InputMode::Normal;
        self.todo_input.clear();
        self.worktree_input.clear();
        self.input_step = 0;
        self.error_message = None;
    }

    fn submit_input(&mut self) -> Result<()> {
        // Validate fields are not empty
        if !self.can_create_worktree() {
            self.error_message = Some("Description and worktree name cannot be empty".to_string());
            return Ok(());
        }

        // Create worktree
        let worktree_name = self.worktree_input.trim().to_string();
        let todo_description = self.todo_input.trim().to_string();

        // Use worktree name as branch name
        let branch = Some(worktree_name.as_str());

        match git::create_worktree(&worktree_name, branch) {
            Ok(_) => {
                // Create a linked todo
                self.app_config
                    .add_todo(todo_description, worktree_name.clone());
                self.app_config.save()?;
                self.refresh_worktrees()?;
                self.cancel_input();
                // Select the newly created todo (it will be the last one)
                let new_pos = self.app_config.todos.len().saturating_sub(1);
                self.list_state.select(Some(new_pos));
            }
            Err(e) => {
                self.error_message = Some(format!("Failed to create worktree: {e}"));
            }
        }
        Ok(())
    }

    fn update_worktree_name(&mut self) {
        self.worktree_input = Self::dasherize(&self.todo_input);
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
        eprintln!("Error: {err:?}");
    }

    Ok(())
}

fn run_app<B: ratatui::backend::Backend>(terminal: &mut Terminal<B>, app: &mut App) -> Result<()> {
    loop {
        terminal.draw(|f| ui(f, &mut *app))?;

        match event::read()? {
            Event::Key(key) => {
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
                                if i < app.app_config.todos.len() {
                                    let todo = &app.app_config.todos[i];
                                    if let Some(ref worktree_name) = todo.worktree {
                                        // Find the actual worktree
                                        if let Some(worktree) = app
                                            .worktrees
                                            .iter()
                                            .find(|wt| &wt.name == worktree_name)
                                        {
                                            // Exit TUI and start tmux session
                                            disable_raw_mode()?;
                                            return crate::tmux::start_session_with_app(
                                                &worktree.name,
                                                &worktree.path,
                                                &app.app_config,
                                            );
                                        }
                                    }
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
                            app.todo_input.push(c);
                            app.update_worktree_name();
                        }
                        KeyCode::Backspace => {
                            app.todo_input.pop();
                            app.update_worktree_name();
                        }
                        _ => {}
                    },
                }
            }
            Event::Mouse(mouse) => {
                match app.input_mode {
                    InputMode::Normal => {
                        use crossterm::event::{MouseButton, MouseEventKind};
                        if let MouseEventKind::Down(MouseButton::Left) = mouse.kind {
                            // Handle list item clicks
                            if let Some(clicked_index) = app.handle_click(mouse.column, mouse.row) {
                                app.list_state.select(Some(clicked_index));
                            }
                            // Handle New button click
                            if app.is_new_button_clicked(mouse.column, mouse.row) {
                                app.start_create_worktree();
                            }
                        }
                    }
                    InputMode::CreatingWorktree | InputMode::Help | InputMode::ConfirmDelete => {
                        // Mouse events not handled in these modes
                    }
                }
            }
            _ => {}
        }
    }
}

fn ui(f: &mut Frame, app: &mut App) {
    // Main content
    match app.input_mode {
        InputMode::Normal => {
            let chunks = Layout::default()
                .direction(Direction::Vertical)
                .constraints([Constraint::Min(0), Constraint::Length(3)])
                .split(f.area());

            app.list_area = chunks[0];
            render_unified_list(f, app, chunks[0]);

            // Bottom row with help and button
            let bottom_chunks = Layout::default()
                .direction(Direction::Horizontal)
                .constraints([Constraint::Percentage(80), Constraint::Percentage(20)])
                .split(chunks[1]);

            render_help(f, bottom_chunks[0]);
            app.button_area = bottom_chunks[1];
            render_new_button(f, bottom_chunks[1]);
        }
        InputMode::CreatingWorktree => {
            let chunks = Layout::default()
                .direction(Direction::Vertical)
                .constraints([Constraint::Min(0), Constraint::Length(3)])
                .split(f.area());

            render_create_worktree(f, app, chunks[0]);
            render_input_help(f, app, chunks[1]);
        }
        InputMode::Help => {
            let chunks = Layout::default()
                .direction(Direction::Vertical)
                .constraints([Constraint::Min(0), Constraint::Length(3)])
                .split(f.area());

            render_full_help(f, chunks[0]);
            let help_footer = Paragraph::new("Press ? or Esc to close")
                .style(Style::default().fg(Color::Gray))
                .block(Block::default().borders(Borders::ALL));
            f.render_widget(help_footer, chunks[1]);
        }
        InputMode::ConfirmDelete => {
            let chunks = Layout::default()
                .direction(Direction::Vertical)
                .constraints([Constraint::Min(0), Constraint::Length(8)])
                .split(f.area());

            app.list_area = chunks[0];
            render_unified_list(f, app, chunks[0]);
            render_confirm_delete(f, app, chunks[1]);
        }
    }
}

fn render_unified_list(f: &mut Frame, app: &App, area: Rect) {
    use crate::config::TodoStatus;
    use std::collections::HashMap;

    // Create a map of worktree names for quick lookup
    let worktree_names: HashMap<String, &Worktree> = app
        .worktrees
        .iter()
        .map(|wt| (wt.name.clone(), wt))
        .collect();

    let items: Vec<ListItem> = app
        .app_config
        .todos
        .iter()
        .map(|todo| {
            let checkbox = match todo.status {
                TodoStatus::Done => "[✓] ",
                TodoStatus::Pending => "[ ] ",
            };

            let text_style = match todo.status {
                TodoStatus::Done => Style::default().fg(Color::DarkGray),
                TodoStatus::Pending => Style::default().fg(Color::White),
            };

            let worktree_info = if let Some(ref wt_name) = todo.worktree {
                if worktree_names.contains_key(wt_name) {
                    format!(" ({wt_name})")
                } else {
                    format!(" ({wt_name}) [deleted]")
                }
            } else {
                String::new()
            };

            let content = vec![Line::from(vec![
                Span::styled(checkbox, Style::default().fg(Color::Green)),
                Span::styled(&todo.description, text_style),
                Span::styled(worktree_info, Style::default().fg(Color::DarkGray)),
            ])];
            ListItem::new(content)
        })
        .collect();

    let list = List::new(items)
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title("Todos & Worktrees (↑↓/jk to navigate, Tab to toggle, Enter to select)"),
        )
        .highlight_style(
            Style::default()
                .bg(Color::DarkGray)
                .add_modifier(Modifier::BOLD),
        )
        .highlight_symbol(">> ");

    f.render_stateful_widget(list, area, &mut app.list_state.clone());
}

fn render_create_worktree(f: &mut Frame, app: &App, area: Rect) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Length(3),
            Constraint::Min(0),
        ])
        .split(area);

    let todo_input = Paragraph::new(app.todo_input.as_str())
        .style(Style::default().fg(Color::Yellow))
        .block(Block::default().borders(Borders::ALL).title("Description"));
    f.render_widget(todo_input, chunks[0]);

    let worktree_input = Paragraph::new(app.worktree_input.as_str())
        .style(Style::default().fg(Color::DarkGray))
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title("Worktree Name (auto-generated)"),
        );
    f.render_widget(worktree_input, chunks[1]);

    if let Some(error) = &app.error_message {
        let error_widget = Paragraph::new(error.as_str())
            .style(Style::default().fg(Color::Red))
            .block(Block::default().borders(Borders::ALL).title("Error"));
        f.render_widget(error_widget, chunks[2]);
    }
}

fn render_new_button(f: &mut Frame, area: Rect) {
    let button = Paragraph::new("[ New ]")
        .style(
            Style::default()
                .fg(Color::Green)
                .add_modifier(Modifier::BOLD),
        )
        .block(Block::default().borders(Borders::ALL))
        .alignment(ratatui::layout::Alignment::Center);
    f.render_widget(button, area);
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

fn render_input_help(f: &mut Frame, app: &App, area: Rect) {
    let help_text = if app.can_create_worktree() {
        "Enter: Create | Esc: Cancel"
    } else {
        "Type a description to continue | Esc: Cancel"
    };

    let help = Paragraph::new(help_text)
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL));
    f.render_widget(help, area);
}

fn render_full_help(f: &mut Frame, area: Rect) {
    let help_text = vec![
        Line::from(vec![Span::styled(
            "Navigation",
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        )]),
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
        Line::from(vec![Span::styled(
            "Actions",
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        )]),
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
                    Span::styled(
                        "⚠ WARNING: ",
                        Style::default().fg(Color::Red).add_modifier(Modifier::BOLD),
                    ),
                    Span::styled(
                        "This worktree has uncommitted changes!",
                        Style::default().fg(Color::Yellow),
                    ),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::raw("Delete worktree '"),
                    Span::styled(
                        &worktree.name,
                        Style::default()
                            .fg(Color::Cyan)
                            .add_modifier(Modifier::BOLD),
                    ),
                    Span::raw("'?"),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled(
                        "Y",
                        Style::default()
                            .fg(Color::Green)
                            .add_modifier(Modifier::BOLD),
                    ),
                    Span::raw("es (force delete) | "),
                    Span::styled(
                        "N",
                        Style::default().fg(Color::Red).add_modifier(Modifier::BOLD),
                    ),
                    Span::raw("o / Esc"),
                ]),
            ]
        } else {
            vec![
                Line::from(vec![
                    Span::raw("Delete worktree '"),
                    Span::styled(
                        &worktree.name,
                        Style::default()
                            .fg(Color::Cyan)
                            .add_modifier(Modifier::BOLD),
                    ),
                    Span::raw("'?"),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled(
                        "Y",
                        Style::default()
                            .fg(Color::Green)
                            .add_modifier(Modifier::BOLD),
                    ),
                    Span::raw("es | "),
                    Span::styled(
                        "N",
                        Style::default().fg(Color::Red).add_modifier(Modifier::BOLD),
                    ),
                    Span::raw("o / Esc"),
                ]),
            ]
        };

        let confirm = Paragraph::new(message)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title("Confirm Delete"),
            )
            .style(Style::default());

        f.render_widget(confirm, area);
    }
}
