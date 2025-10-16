use anyhow::{anyhow, Context, Result};
use std::path::PathBuf;
use std::process::Command;

#[derive(Debug, Clone)]
pub struct Worktree {
    pub name: String,
    pub path: PathBuf,
    pub branch: String,
}

/// Get list of all git worktrees
pub fn list_worktrees() -> Result<Vec<Worktree>> {
    let output = Command::new("git")
        .args(["worktree", "list", "--porcelain"])
        .output()
        .context("Failed to execute git worktree list")?;

    if !output.status.success() {
        return Err(anyhow!(
            "git worktree list failed: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    let stdout = String::from_utf8(output.stdout)?;
    parse_worktrees(&stdout)
}

fn parse_worktrees(output: &str) -> Result<Vec<Worktree>> {
    let mut worktrees = Vec::new();
    let mut current_path: Option<PathBuf> = None;
    let mut current_branch: Option<String> = None;

    for line in output.lines() {
        if line.starts_with("worktree ") {
            current_path = Some(PathBuf::from(line.trim_start_matches("worktree ")));
        } else if line.starts_with("branch ") {
            current_branch = Some(
                line.trim_start_matches("branch ")
                    .trim_start_matches("refs/heads/")
                    .to_string(),
            );
        } else if line.is_empty() {
            if let (Some(path), Some(branch)) = (current_path.take(), current_branch.take()) {
                let name = path
                    .file_name()
                    .and_then(|n| n.to_str())
                    .unwrap_or("unknown")
                    .to_string();

                worktrees.push(Worktree { name, path, branch });
            }
        }
    }

    // Handle the last worktree if there's no trailing empty line
    if let (Some(path), Some(branch)) = (current_path, current_branch) {
        let name = path
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("unknown")
            .to_string();

        worktrees.push(Worktree { name, path, branch });
    }

    Ok(worktrees)
}

/// Find a worktree by name
pub fn find_worktree(name: &str) -> Result<Worktree> {
    let worktrees = list_worktrees()?;
    worktrees
        .into_iter()
        .find(|wt| wt.name == name)
        .ok_or_else(|| anyhow!("Worktree '{}' not found", name))
}

/// Create a new worktree
pub fn create_worktree(name: &str, branch: Option<&str>) -> Result<PathBuf> {
    // Get the root of the git repository
    let output = Command::new("git")
        .args(["rev-parse", "--show-toplevel"])
        .output()
        .context("Failed to get git root")?;

    if !output.status.success() {
        return Err(anyhow!("Not in a git repository"));
    }

    let git_root = PathBuf::from(String::from_utf8(output.stdout)?.trim());
    let worktree_path = git_root.parent().unwrap_or(&git_root).join(name);

    let mut cmd = Command::new("git");
    cmd.args(["worktree", "add"]);

    if let Some(b) = branch {
        cmd.arg("-b").arg(b);
    }

    cmd.arg(&worktree_path);

    let output = cmd.output().context("Failed to create worktree")?;

    if !output.status.success() {
        return Err(anyhow!(
            "Failed to create worktree: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    Ok(worktree_path)
}

/// Jump to a worktree and start tmux session
pub fn jump_to_worktree(name: &str) -> Result<()> {
    let worktree = find_worktree(name)?;
    crate::tmux::start_session(name, &worktree.path)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_worktrees() {
        let output = r#"worktree /Users/test/project
HEAD 1234567890abcdef
branch refs/heads/main

worktree /Users/test/project-feature
HEAD abcdef1234567890
branch refs/heads/feature

"#;

        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 2);
        assert_eq!(worktrees[0].name, "project");
        assert_eq!(worktrees[0].branch, "main");
        assert_eq!(worktrees[1].name, "project-feature");
        assert_eq!(worktrees[1].branch, "feature");
    }
}
