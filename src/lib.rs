// Library interface for lfg
// This allows integration tests and potential future uses as a library

pub mod cli;
pub mod config;
pub mod git;
pub mod tmux;
pub mod tui;

// Re-export commonly used types for convenience
pub use git::Worktree;
pub use config::{Config, TmuxWindow};
