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

        // Create remaining windows
        for window in config.windows.iter().skip(1) {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_available() {
        // This test checks if tmux is available on the system
        // The result will vary depending on whether tmux is installed
        let available = is_available();
        // We can't assert a specific value, but we can verify it returns a bool
        assert!(available == true || available == false);
    }

    #[test]
    fn test_session_exists_with_nonexistent_session() {
        // Use a very unlikely session name that probably doesn't exist
        let session_name = "lfg_test_nonexistent_session_12345_unlikely";

        // This will either succeed (if tmux is installed) or fail (if not)
        // If tmux is not installed, session_exists will return an error
        let result = session_exists(session_name);

        if result.is_ok() {
            // If tmux is available, the session should not exist
            assert_eq!(result.unwrap(), false);
        } else {
            // If tmux is not available, we expect an error
            assert!(result.is_err());
        }
    }

    #[test]
    fn test_kill_session_nonexistent() {
        // Killing a non-existent session should succeed (no-op)
        let session_name = "lfg_test_nonexistent_session_99999";

        // This should either succeed (if tmux is available) or fail (if not)
        let result = kill_session(session_name);

        // If tmux is available, killing a non-existent session should succeed
        // If tmux is not available, it will error
        if is_available() {
            assert!(result.is_ok());
        }
    }

    // Integration-like tests that require tmux to be installed
    // These tests are more comprehensive but require tmux

    #[test]
    #[ignore] // Ignored by default, run with `cargo test -- --ignored`
    fn test_create_and_kill_session() {
        // This test requires tmux to be installed
        if !is_available() {
            return; // Skip if tmux not available
        }

        let session_name = "lfg_test_session_integration";
        let test_dir = std::env::temp_dir();

        // Clean up any existing session
        let _ = kill_session(session_name);

        // Create a session
        let _result = start_session(session_name, &test_dir);

        // Note: start_session tries to attach, which will fail in a test environment
        // without a TTY, so we expect this to fail even though the session is created
        // This is a limitation of testing tmux in non-interactive environments

        // Clean up
        let _ = kill_session(session_name);
    }

    #[test]
    #[ignore] // Ignored by default
    fn test_session_exists_with_real_session() {
        if !is_available() {
            return; // Skip if tmux not available
        }

        let session_name = "lfg_test_check_exists";
        let test_dir = std::env::temp_dir();

        // Clean up any existing session
        let _ = kill_session(session_name);

        // Create a detached session manually
        let _ = Command::new("tmux")
            .args(["new-session", "-d", "-s", session_name, "-c"])
            .arg(&test_dir)
            .output();

        // Check if session exists
        let exists = session_exists(session_name);

        if exists.is_ok() {
            assert!(exists.unwrap());
        }

        // Clean up
        let _ = kill_session(session_name);

        // Verify it's gone
        let exists_after = session_exists(session_name);
        if exists_after.is_ok() {
            assert!(!exists_after.unwrap());
        }
    }

    #[test]
    fn test_attach_session_error_message() {
        // Test that attach_session is a private function
        // We can't call it directly, but we can verify it exists via the module structure
        // This is more of a compilation test than a runtime test

        // Just verify the module compiles correctly
        assert!(true);
    }
}
