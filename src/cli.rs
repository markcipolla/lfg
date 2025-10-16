use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "lfg")]
#[command(about = "Git worktree manager with tmux integration", long_about = None)]
pub struct Args {
    /// Jump directly to a worktree by name
    pub worktree: Option<String>,
}
