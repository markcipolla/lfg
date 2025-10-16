use anyhow::{anyhow, Context, Result};
use std::path::PathBuf;
use std::process::Command;

#[derive(Debug, Clone)]
pub struct Worktree {
    pub name: String,
    pub path: PathBuf,
    #[allow(dead_code)]
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

/// Get the root directory of the git repository
pub fn get_git_root() -> Result<PathBuf> {
    let output = Command::new("git")
        .args(["rev-parse", "--show-toplevel"])
        .output()
        .context("Failed to get git root")?;

    if !output.status.success() {
        return Err(anyhow!("Not in a git repository"));
    }

    let git_root = PathBuf::from(String::from_utf8(output.stdout)?.trim());
    Ok(git_root)
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
    let git_root = get_git_root()?;
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

/// Get the current worktree if the current directory is inside one
pub fn get_current_worktree() -> Result<Option<String>> {
    let current_dir = std::env::current_dir().context("Failed to get current directory")?;
    let worktrees = list_worktrees()?;

    // Find if current directory is within any worktree
    for worktree in worktrees {
        if current_dir.starts_with(&worktree.path) {
            return Ok(Some(worktree.name));
        }
    }

    Ok(None)
}

/// Jump to a worktree and start tmux session
pub fn jump_to_worktree(name: &str) -> Result<()> {
    let worktree = find_worktree(name)?;
    crate::tmux::start_session(name, &worktree.path)
}

/// Check if a worktree has uncommitted changes
pub fn is_worktree_dirty(path: &PathBuf) -> Result<bool> {
    let output = Command::new("git")
        .args(["-C", path.to_str().unwrap(), "status", "--porcelain"])
        .output()
        .context("Failed to check worktree status")?;

    if !output.status.success() {
        return Err(anyhow!(
            "git status failed: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    // If output is not empty, there are uncommitted changes
    Ok(!output.stdout.is_empty())
}

/// Delete a worktree
pub fn delete_worktree(path: &PathBuf, force: bool) -> Result<()> {
    let mut cmd = Command::new("git");
    cmd.args(["worktree", "remove"]);

    if force {
        cmd.arg("--force");
    }

    cmd.arg(path);

    let output = cmd.output().context("Failed to delete worktree")?;

    if !output.status.success() {
        return Err(anyhow!(
            "Failed to delete worktree: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    Ok(())
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

    #[test]
    fn test_parse_worktrees_without_trailing_newline() {
        let output = r#"worktree /Users/test/project
HEAD 1234567890abcdef
branch refs/heads/main"#;

        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 1);
        assert_eq!(worktrees[0].name, "project");
        assert_eq!(worktrees[0].branch, "main");
        assert_eq!(worktrees[0].path, PathBuf::from("/Users/test/project"));
    }

    #[test]
    fn test_parse_worktrees_empty_output() {
        let output = "";
        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 0);
    }

    #[test]
    fn test_parse_worktrees_single_worktree() {
        let output = r#"worktree /home/user/repo
HEAD 1234567890abcdef
branch refs/heads/develop

"#;

        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 1);
        assert_eq!(worktrees[0].name, "repo");
        assert_eq!(worktrees[0].branch, "develop");
        assert_eq!(worktrees[0].path, PathBuf::from("/home/user/repo"));
    }

    #[test]
    fn test_parse_worktrees_branch_without_refs_heads() {
        let output = r#"worktree /Users/test/project
HEAD 1234567890abcdef
branch main

"#;

        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 1);
        assert_eq!(worktrees[0].branch, "main");
    }

    #[test]
    fn test_parse_worktrees_multiple_worktrees_no_trailing_newline() {
        let output = r#"worktree /Users/test/project
HEAD 1234567890abcdef
branch refs/heads/main

worktree /Users/test/project-feature
HEAD abcdef1234567890
branch refs/heads/feature"#;

        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 2);
        assert_eq!(worktrees[0].name, "project");
        assert_eq!(worktrees[0].branch, "main");
        assert_eq!(worktrees[1].name, "project-feature");
        assert_eq!(worktrees[1].branch, "feature");
    }

    #[test]
    fn test_parse_worktrees_complex_paths() {
        let output = r#"worktree /Users/test/my-awesome-project
HEAD 1234567890abcdef
branch refs/heads/feature/new-feature

worktree /home/user/projects/repo_with_underscores
HEAD abcdef1234567890
branch refs/heads/bugfix/fix-123

"#;

        let worktrees = parse_worktrees(output).unwrap();
        assert_eq!(worktrees.len(), 2);
        assert_eq!(worktrees[0].name, "my-awesome-project");
        assert_eq!(worktrees[0].branch, "feature/new-feature");
        assert_eq!(worktrees[1].name, "repo_with_underscores");
        assert_eq!(worktrees[1].branch, "bugfix/fix-123");
    }

    #[test]
    fn test_worktree_struct_clone() {
        let worktree = Worktree {
            name: "test".to_string(),
            path: PathBuf::from("/test/path"),
            branch: "main".to_string(),
        };

        let cloned = worktree.clone();
        assert_eq!(cloned.name, "test");
        assert_eq!(cloned.path, PathBuf::from("/test/path"));
        assert_eq!(cloned.branch, "main");
    }

    #[test]
    fn test_worktree_struct_debug() {
        let worktree = Worktree {
            name: "test".to_string(),
            path: PathBuf::from("/test/path"),
            branch: "main".to_string(),
        };

        let debug_str = format!("{:?}", worktree);
        assert!(debug_str.contains("test"));
        assert!(debug_str.contains("main"));
    }
}
