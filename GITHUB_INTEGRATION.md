# GitHub Projects Integration

LFG now supports integrating with GitHub Projects as an alternative to local YAML storage for todos!

## Features

- **Choose your storage backend** during initialization
- **Automatic sync** between local cache and GitHub Projects
- **Seamless PR creation** - todos already exist in GitHub when you create PRs
- **Team collaboration** - multiple developers can see shared project progress
- **Offline support** - works with cached data when offline

## Setup

### Prerequisites

1. Install and authenticate with GitHub CLI:
   ```bash
   gh auth login
   ```

2. Create a GitHub Project (Projects V2) for your repository at:
   ```
   https://github.com/OWNER/REPO/projects
   ```

### Initialization

When you first run LFG in a new repository, you'll see the initialization wizard:

```bash
$ lfg

Welcome to LFG initialization!

Enter project name (default: current directory name): my-project

Choose todo storage backend:
  1. Local YAML (stored in repository)
  2. GitHub Projects (synced with GitHub)

Choice [1-2] (default: 1): 2

Setting up GitHub Projects integration...

Repository: myorg/myrepo

Fetching available projects...

Available projects:
  1. Development Backlog (Project #1)
  2. Sprint Planning (Project #2)

Select project [1-2]: 1

Selected: Development Backlog

Configuration created successfully!

Todos will be synced with GitHub Project #1 in myorg/myrepo
```

## Configuration

### Local YAML Backend (Default)

```yaml
name: myapp
worktree_naming: Add feature
storage_backend: local
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

### GitHub Projects Backend

```yaml
name: myapp
worktree_naming: Add feature
storage_backend:
  github:
    owner: myorg
    repo: myrepo
    project_number: 1
todos: []  # Synced from GitHub on load
windows:
  - name: editor
    command: null
  - name: server
    command: omnara --dangerously-skip-permissions
  - name: shell
    command: null
```

## How It Works

### Syncing Behavior

1. **On app start**: Fetches todos from GitHub Projects and caches them locally
2. **Creating a todo**: Creates both a local cache entry and a GitHub draft issue in the project
3. **Marking as done**: Updates local status and syncs to GitHub (moves to "Done" column)
4. **Offline mode**: Uses local cache when GitHub is unavailable

### GitHub Projects Structure

LFG expects your GitHub Project to have:
- **Status field**: Used to track "Pending" vs "Done" todos
- **Optional Worktree field**: Custom field to track which worktree is associated with each todo

### Creating PRs with GitHub Integration

When using GitHub Projects:

1. Create a worktree in LFG (creates a todo in GitHub Project)
2. Make your changes
3. The todo already exists in GitHub, making it easy to reference in PR description
4. When you delete the worktree, LFG marks the todo as done in GitHub

## Implementation Details

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                    LFG Application                   │
├─────────────────────────────────────────────────────┤
│                                                      │
│  ┌──────────────┐      ┌──────────────────────┐   │
│  │ AppConfig    │◄─────┤ StorageBackend       │   │
│  │              │      │ - Local              │   │
│  │ - todos      │      │ - GitHub {           │   │
│  │ - windows    │      │     owner,           │   │
│  │ - backend    │      │     repo,            │   │
│  └──────────────┘      │     project_number   │   │
│         │              │   }                  │   │
│         │              └──────────────────────┘   │
│         ▼                                          │
│  ┌──────────────────────────────────────────┐     │
│  │         Sync Logic                      │     │
│  │  - Load: Fetch from GitHub → Cache     │     │
│  │  - Add: Cache + GitHub API call        │     │
│  │  - Update: Cache + GitHub API call     │     │
│  └──────────────────────────────────────────┘     │
│                    │                               │
└────────────────────┼───────────────────────────────┘
                     │
                     ▼
         ┌────────────────────────┐
         │   GitHub API (gh CLI)  │
         │   - GraphQL API v2     │
         │   - ProjectsV2         │
         └────────────────────────┘
```

### API Calls

LFG uses the `gh` CLI to interact with GitHub's GraphQL API:

- **List projects**: Query `projectsV2` for repository
- **Fetch todos**: Query project items with status fields
- **Add todo**: Mutation `addProjectV2DraftIssue`
- **Update status**: Update item field values (TODO: to be implemented)

## Future Enhancements

- [ ] Full bidirectional sync (detect changes from GitHub)
- [ ] Auto-link todos to PRs when created
- [ ] Support for custom field mapping (beyond Status and Worktree)
- [ ] Sync on PR merge (auto-mark todo as done)
- [ ] Support for GitHub Issues in addition to draft issues

## Troubleshooting

### "GitHub CLI is not authenticated"

Run `gh auth login` and follow the prompts.

### "No projects found for this repository"

Create a Project V2 at `https://github.com/OWNER/REPO/projects`.

### Todos not syncing

Check that:
1. You have network connectivity
2. `gh` CLI is properly authenticated
3. You have access to the repository and project
4. The project number in `lfg-config.yaml` is correct

## Switching Backends

To switch from Local to GitHub (or vice versa):

1. Delete your `lfg-config.yaml`
2. Run `lfg` to trigger the init wizard
3. Choose your preferred backend

Note: Todos won't be automatically migrated between backends.
