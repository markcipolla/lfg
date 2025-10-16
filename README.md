# LFG - Git Worktree Manager

A Rust TUI (Terminal User Interface) application for managing git worktrees with integrated tmux session management.

## Features

- Interactive TUI for browsing and selecting git worktrees
- Create new worktrees directly from the interface
- Direct jump to worktrees via command-line argument
- Automatic tmux session creation with configurable windows
- Persistent configuration for tmux window setup

## Installation

```bash
cargo install --path .
```

Or build from source:

```bash
cargo build --release
# Binary will be at target/release/lfg
```

## Usage

### Interactive Mode

Launch the TUI by running `lfg` without arguments:

```bash
lfg
```

**Navigation:**
- `↑`/`↓` or `j`/`k`: Navigate through worktrees
- `Enter`: Select worktree and start tmux session
- `n` or `c`: Create new worktree
- `r`: Refresh worktree list
- `q` or `Esc`: Quit

### Direct Jump Mode

Jump directly to a worktree and start its tmux session:

```bash
lfg <worktree-name>
```

## Configuration

Configuration is stored at `~/.config/lfg/config.toml`. On first run, a default configuration will be created.

### Default Configuration

```toml
[[windows]]
name = "rails"
command = "bin/rails s"

[[windows]]
name = "tailwind"
command = "bin/rails tailwind:watch"

[[windows]]
name = "omnara"
command = "omnara --dangerously-skip-permissions"

[[windows]]
name = "shell"
```

### Customizing Windows

Edit the config file to customize the tmux windows that get created:

```toml
[[windows]]
name = "editor"
command = "nvim"

[[windows]]
name = "server"
command = "npm run dev"

[[windows]]
name = "logs"
command = "tail -f logs/development.log"

[[windows]]
name = "shell"
# No command = just opens a shell
```

## How It Works

1. **Worktree Discovery**: LFG scans your git worktrees using `git worktree list`
2. **Selection**: Choose a worktree from the TUI or specify it via command line
3. **Tmux Session**: Creates a tmux session named after the worktree
4. **Window Setup**: Creates configured tmux windows in the worktree directory
5. **Attachment**: Attaches you to the tmux session

## Requirements

- Rust 1.70+
- Git with worktree support
- tmux

## Comparison with Bash Function

This tool replaces the `lfgw` bash function with a more robust solution:

- **TUI Interface**: Visual selection instead of memorizing worktree names
- **Worktree Creation**: Create new worktrees without leaving the tool
- **Configuration**: Persistent, editable config instead of hardcoded commands
- **Error Handling**: Better error messages and validation
- **Cross-platform**: Works on any platform with Rust support

## License

MIT
