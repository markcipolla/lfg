use anyhow::{anyhow, Context, Result};
use std::path::Path;
use std::process::{Command, Stdio};

use crate::config::Config;

/// Check if tmux is available
pub fn is_available() -> bool {
    Command::new("tmux")
        .arg("-V")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}

/// Check if a tmux session exists
pub fn session_exists(name: &str) -> Result<bool> {
    let output = Command::new("tmux")
        .args(["has-session", "-t", name])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .context("Failed to check tmux session")?;

    Ok(output.success())
}

/// Set the terminal window title for a tmux window
fn set_window_title(session_name: &str, window_index: usize, title: &str) -> Result<()> {
    // Enable setting terminal titles for this session
    Command::new("tmux")
        .args(["set-option", "-t", session_name, "set-titles", "on"])
        .output()
        .context("Failed to enable terminal titles")?;

    // Set the title string for the specific window
    let target = format!("{}:{}", session_name, window_index);
    Command::new("tmux")
        .args([
            "set-option",
            "-t",
            &target,
            "set-titles-string",
            title,
        ])
        .output()
        .context("Failed to set window title")?;

    Ok(())
}

/// Start a new tmux session with configured windows
pub fn start_session(session_name: &str, worktree_path: &Path) -> Result<()> {
    if !is_available() {
        return Err(anyhow!("tmux is not installed or not in PATH"));
    }

    // Check if session already exists
    if session_exists(session_name)? {
        // Attach to existing session
        attach_session(session_name)?;
        return Ok(());
    }

    let config = Config::load()?;
    let path_str = worktree_path
        .to_str()
        .ok_or_else(|| anyhow!("Invalid worktree path"))?;

    // Create first window with command or empty
    if let Some(first_window) = config.windows.first() {
        let mut cmd = Command::new("tmux");
        cmd.args(["new-session", "-d", "-s", session_name, "-c", path_str]);
        cmd.args(["-n", &first_window.name]);

        if let Some(command) = &first_window.command {
            cmd.arg(command);
        }

        let output = cmd.output().context("Failed to create tmux session")?;

        if !output.status.success() {
            return Err(anyhow!(
                "Failed to create tmux session: {}",
                String::from_utf8_lossy(&output.stderr)
            ));
        }

        // Set the terminal title for the first window
        set_window_title(session_name, 0, &first_window.name)?;

        // Create remaining windows
        for (index, window) in config.windows.iter().skip(1).enumerate() {
            let mut cmd = Command::new("tmux");
            cmd.args(["new-window", "-t", session_name, "-c", path_str]);
            cmd.args(["-n", &window.name]);

            if let Some(command) = &window.command {
                cmd.arg(command);
            }

            let output = cmd.output().context("Failed to create tmux window")?;

            if !output.status.success() {
                eprintln!(
                    "Warning: Failed to create window {}: {}",
                    window.name,
                    String::from_utf8_lossy(&output.stderr)
                );
            } else {
                // Set the terminal title for this window (index + 1 because we skipped the first)
                if let Err(e) = set_window_title(session_name, index + 1, &window.name) {
                    eprintln!("Warning: Failed to set title for window {}: {}", window.name, e);
                }
            }
        }
    }

    // Attach to the session
    attach_session(session_name)?;

    Ok(())
}

/// Attach to an existing tmux session
fn attach_session(session_name: &str) -> Result<()> {
    let status = Command::new("tmux")
        .args(["attach-session", "-t", session_name])
        .status()
        .context("Failed to attach to tmux session")?;

    if !status.success() {
        return Err(anyhow!("Failed to attach to tmux session"));
    }

    Ok(())
}

/// Kill a tmux session
#[allow(dead_code)]
pub fn kill_session(session_name: &str) -> Result<()> {
    if !session_exists(session_name)? {
        return Ok(());
    }

    let output = Command::new("tmux")
        .args(["kill-session", "-t", session_name])
        .output()
        .context("Failed to kill tmux session")?;

    if !output.status.success() {
        return Err(anyhow!(
            "Failed to kill tmux session: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    Ok(())
}
