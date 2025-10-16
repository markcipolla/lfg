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
}

struct App {
    worktrees: Vec<Worktree>,
    list_state: ListState,
    input_mode: InputMode,
    input: String,
    branch_input: String,
    input_step: usize, // 0 = name, 1 = branch
    error_message: Option<String>,
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
        })
    }

    fn next(&mut self) {
        if self.worktrees.is_empty() {
            return;
        }
        let i = match self.list_state.selected() {
            Some(i) => {
                if i >= self.worktrees.len() - 1 {
                    0
                } else {
                    i + 1
                }
            }
            None => 0,
        };
        self.list_state.select(Some(i));
    }

    fn previous(&mut self) {
        if self.worktrees.is_empty() {
            return;
        }
        let i = match self.list_state.selected() {
            Some(i) => {
                if i == 0 {
                    self.worktrees.len() - 1
                } else {
                    i - 1
                }
            }
            None => 0,
        };
        self.list_state.select(Some(i));
    }

    fn refresh_worktrees(&mut self) -> Result<()> {
        self.worktrees = git::list_worktrees()?;
        if !self.worktrees.is_empty() && self.list_state.selected().is_none() {
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
                    KeyCode::Char('n') | KeyCode::Char('c') => app.start_create_worktree(),
                    KeyCode::Char('r') => {
                        app.refresh_worktrees()?;
                    }
                    KeyCode::Enter => {
                        if let Some(i) = app.list_state.selected() {
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
    }
}

fn render_worktree_list(f: &mut Frame, app: &App, area: Rect) {
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
                .title("Worktrees (↑↓/jk to navigate, Enter to select)"),
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
    let help = Paragraph::new("q/Esc: Quit | n/c: New worktree | r: Refresh | Enter: Select")
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL));
    f.render_widget(help, area);
}

fn render_input_help(f: &mut Frame, area: Rect) {
    let help = Paragraph::new("Enter: Next/Create | Esc: Cancel")
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL));
    f.render_widget(help, area);
}
