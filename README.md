# LFG - Git Worktree Manager

A Go TUI (Terminal User Interface) application built with Bubble Tea for managing git worktrees with integrated tmux session management and optional GitHub Projects integration.

## Features

- Interactive TUI for browsing and selecting git worktrees
- **Per-repository configuration with YAML**
- **Automatic tmux installation check**
- **Customizable worktree naming per repository**
- **Repository-specific todo lists displayed in the UI**
- Create new worktrees directly from the interface
- Direct jump to worktrees via command-line argument
- Automatic tmux session creation with configurable windows
- Repository-specific configuration stored in `lfg-config.yaml`

## Installation

Build from source:

```bash
go build -o lfg .
```

Or install directly:

```bash
go install github.com/markcipolla/lfg@latest
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
- `n` or `c`: Create new worktree (creates linked todo)
- `d`: Close worktree and mark todo as done
- `r`: Refresh worktree list
- `q` or `Esc`: Quit

### Direct Jump Mode

Jump directly to a worktree and start its tmux session:

```bash
lfg <worktree-name>
```

## Configuration

LFG uses a repository-specific configuration file called `lfg-config.yaml` stored in the **root of your git repository**.

When you run `lfg` for the first time in a repository, it will automatically create a default `lfg-config.yaml` with sensible defaults.

### Configuration File Location

The config file is always located at: `<git-repo-root>/lfg-config.yaml`

### Configuration Options

Each repository's config can specify:
- **`name`**: Repository/project name
- **`worktree_naming`**: Default name template for new worktrees (pre-filled when creating worktrees)
- **`todos`**: List of tasks linked to worktrees with status tracking
  - `description`: The task description
  - `status`: `pending` or `done`
  - `worktree`: The linked worktree name (optional)
- **`windows`**: Tmux windows and commands to run in each window

### Example Configuration

See `lfg-config.example.yaml` for a complete example:

```yaml
name: myapp
worktree_naming: Add feature
todos:
  - description: Implement login feature
    status: done
    worktree: myapp-login
  - description: Add user profile page
    status: pending
    worktree: myapp-profile
windows:
  - name: editor
    command: null
  - name: server
    command: omnara --dangerously-skip-permissions
  - name: shell
    command: null
```

### Customizing Your Config

1. Run `lfg` in your repository (creates default config if it doesn't exist)
2. Edit `lfg-config.yaml` in your repo root
3. Customize:
   - `worktree_naming`: The default name when creating new worktrees
   - `todos`: Your workflow checklist items (can be empty initially)
   - `windows`: Tmux windows with project-specific commands
4. Commit the config to your repository so your team can use the same setup!

### Worktree & Todo Workflow

LFG automatically links worktrees with todos for better task tracking:

1. **Creating a worktree**: Press `n` or `c` to create a new worktree
   - The worktree name is pre-filled with your `worktree_naming` template
   - A new todo is automatically created and linked to the worktree
   - The todo starts with `pending` status

2. **Working on a worktree**: Press `Enter` to launch your tmux session
   - All configured windows are created with your custom commands
   - The todo remains in `pending` status while you work

3. **Closing a worktree**: Press `d` to close and clean up
   - The worktree is deleted from disk
   - The linked todo is marked as `done` automatically
   - The config is saved with the updated todo status

This workflow helps you track what you're working on and maintain a history of completed work!

## How It Works

1. **Tmux Check**: LFG verifies that tmux is installed before proceeding
2. **Config Loading**: Loads `lfg-config.yaml` from your git repository root (creates default if missing)
3. **Worktree Discovery**: Scans your git worktrees using `git worktree list`
4. **Selection**: Choose a worktree from the TUI or specify it via command line
5. **Tmux Session**: Creates a tmux session named after the worktree
6. **Window Setup**: Creates configured tmux windows in the worktree directory with repository-specific commands
7. **Attachment**: Attaches you to the tmux session

## Requirements

- Go 1.20+
- Git with worktree support
- tmux (automatically checked at runtime)
- gh CLI (optional, for GitHub Projects integration)

## Why Go + Bubble Tea?

This was rewritten from Rust to Go with Bubble Tea because:

- **Charming UI**: Bubble Tea provides a delightful, smooth TUI experience
- **Simpler**: Go's straightforward concurrency and error handling
- **Fast builds**: Much faster compile times than Rust
- **Great ecosystem**: Charm's suite of tools (Bubble Tea, Lipgloss, Bubbles) sparks joy
- **Easy deployment**: Single binary, no runtime dependencies

## License

MIT
