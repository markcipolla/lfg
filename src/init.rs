use anyhow::{Context, Result, anyhow};
use std::io::{self, Write};
use crate::config::{AppConfig, StorageBackend};
use crate::github::{self, GitHubClient};

pub fn run_init_wizard() -> Result<AppConfig> {
    println!("Welcome to LFG initialization!");
    println!();

    // Get project name
    print!("Enter project name (default: current directory name): ");
    io::stdout().flush()?;
    let mut project_name = String::new();
    io::stdin().read_line(&mut project_name)?;
    let project_name = project_name.trim();

    let project_name = if project_name.is_empty() {
        std::env::current_dir()?
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("default")
            .to_string()
    } else {
        project_name.to_string()
    };

    println!();
    println!("Choose todo storage backend:");
    println!("  1. Local YAML (stored in repository)");
    println!("  2. GitHub Projects (synced with GitHub)");
    println!();
    print!("Choice [1-2] (default: 1): ");
    io::stdout().flush()?;

    let mut choice = String::new();
    io::stdin().read_line(&mut choice)?;
    let choice = choice.trim();

    let storage_backend = match choice {
        "2" => {
            println!();
            println!("Setting up GitHub Projects integration...");
            println!();

            // Check if gh CLI is authenticated
            if !GitHubClient::is_authenticated()? {
                return Err(anyhow!(
                    "GitHub CLI is not authenticated. Please run 'gh auth login' first."
                ));
            }

            // Get repository info
            let (owner, repo) = github::get_repo_info()
                .context("Failed to get repository info. Make sure you're in a git repository with a GitHub remote.")?;

            println!("Repository: {}/{}", owner, repo);
            println!();

            // List available projects
            println!("Fetching available projects...");
            let projects = GitHubClient::list_projects(&owner, &repo)?;

            if projects.is_empty() {
                println!("No projects found for this repository.");
                println!("Please create a project at https://github.com/{}/{}/projects", owner, repo);
                return Err(anyhow!("No projects available"));
            }

            println!();
            println!("Available projects:");
            for (i, project) in projects.iter().enumerate() {
                println!("  {}. {} (Project #{})", i + 1, project.title, project.number);
            }
            println!();

            print!("Select project [1-{}]: ", projects.len());
            io::stdout().flush()?;

            let mut project_choice = String::new();
            io::stdin().read_line(&mut project_choice)?;
            let project_choice: usize = project_choice
                .trim()
                .parse()
                .context("Invalid project selection")?;

            if project_choice < 1 || project_choice > projects.len() {
                return Err(anyhow!("Invalid project selection"));
            }

            let selected_project = &projects[project_choice - 1];
            println!();
            println!("Selected: {}", selected_project.title);

            StorageBackend::Github {
                owner,
                repo,
                project_number: selected_project.number,
            }
        }
        _ => StorageBackend::Local,
    };

    println!();
    println!("Configuration created successfully!");
    println!();

    match &storage_backend {
        StorageBackend::Local => {
            println!("Todos will be stored locally in lfg-config.yaml");
        }
        StorageBackend::Github { owner, repo, project_number } => {
            println!("Todos will be synced with GitHub Project #{} in {}/{}", project_number, owner, repo);
        }
    }

    let config = AppConfig {
        name: project_name,
        worktree_naming: "Add feature".to_string(),
        storage_backend,
        todos: vec![],
        windows: vec![
            crate::config::TmuxWindow {
                name: "editor".to_string(),
                command: None,
            },
            crate::config::TmuxWindow {
                name: "server".to_string(),
                command: Some("omnara --dangerously-skip-permissions".to_string()),
            },
            crate::config::TmuxWindow {
                name: "shell".to_string(),
                command: None,
            },
        ],
    };

    Ok(config)
}
