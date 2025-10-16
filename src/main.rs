use anyhow::Result;
use clap::Parser;

mod cli;
mod config;
mod git;
mod github;
mod init;
mod tmux;
mod tui;

use cli::Args;

fn main() -> Result<()> {
    let args = Args::parse();

    if let Some(worktree_name) = args.worktree {
        // Direct jump to worktree
        git::jump_to_worktree(&worktree_name)?;
    } else {
        // Show TUI for worktree selection
        tui::run()?;
    }

    Ok(())
}
