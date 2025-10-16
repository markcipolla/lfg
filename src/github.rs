use anyhow::{Context, Result, anyhow};
use serde::{Deserialize, Serialize};
use std::process::Command;

use crate::config::{Todo, TodoStatus};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GitHubProject {
    pub id: String,
    pub number: u32,
    pub title: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectItem {
    pub id: String,
    pub content: ProjectItemContent,
    pub field_values: Vec<FieldValue>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectItemContent {
    pub title: String,
    pub body: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FieldValue {
    pub field: String,
    pub value: String,
}

pub struct GitHubClient {
    owner: String,
    repo: String,
    project_number: u32,
}

impl GitHubClient {
    pub fn new(owner: String, repo: String, project_number: u32) -> Self {
        Self {
            owner,
            repo,
            project_number,
        }
    }

    /// Check if gh CLI is authenticated
    pub fn is_authenticated() -> Result<bool> {
        let output = Command::new("gh")
            .args(&["auth", "status"])
            .output()
            .context("Failed to check gh auth status")?;

        Ok(output.status.success())
    }

    /// List available projects for the repository
    pub fn list_projects(owner: &str, repo: &str) -> Result<Vec<GitHubProject>> {
        let query = format!(
            r#"
            query {{
              repository(owner: "{}", name: "{}") {{
                projectsV2(first: 10) {{
                  nodes {{
                    id
                    number
                    title
                  }}
                }}
              }}
            }}
            "#,
            owner, repo
        );

        let output = Command::new("gh")
            .args(&["api", "graphql", "-f", &format!("query={}", query)])
            .output()
            .context("Failed to list projects")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(anyhow!("Failed to list projects: {}", stderr));
        }

        let response: serde_json::Value = serde_json::from_slice(&output.stdout)
            .context("Failed to parse project list response")?;

        let projects: Vec<GitHubProject> = response["data"]["repository"]["projectsV2"]["nodes"]
            .as_array()
            .context("Invalid project list format")?
            .iter()
            .filter_map(|node| {
                Some(GitHubProject {
                    id: node["id"].as_str()?.to_string(),
                    number: node["number"].as_u64()? as u32,
                    title: node["title"].as_str()?.to_string(),
                })
            })
            .collect();

        Ok(projects)
    }

    /// Get project ID from project number
    fn get_project_id(&self) -> Result<String> {
        let query = format!(
            r#"
            query {{
              repository(owner: "{}", name: "{}") {{
                projectsV2(first: 10) {{
                  nodes {{
                    id
                    number
                  }}
                }}
              }}
            }}
            "#,
            self.owner, self.repo
        );

        let output = Command::new("gh")
            .args(&["api", "graphql", "-f", &format!("query={}", query)])
            .output()
            .context("Failed to get project ID")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(anyhow!("Failed to get project ID: {}", stderr));
        }

        let response: serde_json::Value = serde_json::from_slice(&output.stdout)
            .context("Failed to parse project ID response")?;

        let nodes = response["data"]["repository"]["projectsV2"]["nodes"]
            .as_array()
            .context("Invalid project list format")?;

        for node in nodes {
            if node["number"].as_u64() == Some(self.project_number as u64) {
                return Ok(node["id"]
                    .as_str()
                    .context("Missing project ID")?
                    .to_string());
            }
        }

        Err(anyhow!("Project {} not found", self.project_number))
    }

    /// Fetch all todos from GitHub Project
    pub fn fetch_todos(&self) -> Result<Vec<Todo>> {
        let project_id = self.get_project_id()?;

        let query = format!(
            r#"
            query {{
              node(id: "{}") {{
                ... on ProjectV2 {{
                  items(first: 100) {{
                    nodes {{
                      id
                      content {{
                        ... on Issue {{
                          title
                          body
                        }}
                        ... on DraftIssue {{
                          title
                          body
                        }}
                      }}
                      fieldValues(first: 10) {{
                        nodes {{
                          ... on ProjectV2ItemFieldSingleSelectValue {{
                            field {{
                              ... on ProjectV2SingleSelectField {{
                                name
                              }}
                            }}
                            name
                          }}
                          ... on ProjectV2ItemFieldTextValue {{
                            field {{
                              ... on ProjectV2Field {{
                                name
                              }}
                            }}
                            text
                          }}
                        }}
                      }}
                    }}
                  }}
                }}
              }}
            }}
            "#,
            project_id
        );

        let output = Command::new("gh")
            .args(&["api", "graphql", "-f", &format!("query={}", query)])
            .output()
            .context("Failed to fetch project items")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(anyhow!("Failed to fetch project items: {}", stderr));
        }

        let response: serde_json::Value = serde_json::from_slice(&output.stdout)
            .context("Failed to parse project items response")?;

        let items = response["data"]["node"]["items"]["nodes"]
            .as_array()
            .context("Invalid project items format")?;

        let mut todos = Vec::new();

        for item in items {
            let title = item["content"]["title"]
                .as_str()
                .unwrap_or("Untitled")
                .to_string();

            // Look for status field
            let mut status = TodoStatus::Pending;
            let mut worktree_name: Option<String> = None;

            if let Some(field_values) = item["fieldValues"]["nodes"].as_array() {
                for field_value in field_values {
                    if let Some(field_name) = field_value["field"]["name"].as_str() {
                        if field_name == "Status" {
                            if let Some(value) = field_value["name"].as_str() {
                                if value.to_lowercase().contains("done")
                                    || value.to_lowercase().contains("complete")
                                {
                                    status = TodoStatus::Done;
                                }
                            }
                        } else if field_name == "Worktree" {
                            worktree_name = field_value["text"].as_str().map(|s| s.to_string());
                        }
                    }
                }
            }

            todos.push(Todo {
                description: title,
                status,
                worktree: worktree_name,
            });
        }

        Ok(todos)
    }

    /// Add a new todo to GitHub Project
    pub fn add_todo(&self, description: &str, _worktree_name: &str) -> Result<()> {
        let project_id = self.get_project_id()?;

        // First, create a draft issue in the project
        let mutation = format!(
            r#"
            mutation {{
              addProjectV2DraftIssue(input: {{
                projectId: "{}"
                title: "{}"
              }}) {{
                projectItem {{
                  id
                }}
              }}
            }}
            "#,
            project_id,
            description.replace('"', "\\\"")
        );

        let output = Command::new("gh")
            .args(&["api", "graphql", "-f", &format!("query={}", mutation)])
            .output()
            .context("Failed to add project item")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            return Err(anyhow!("Failed to add project item: {}", stderr));
        }

        // TODO: Set the worktree field value
        // This would require getting the field ID first, then updating the item
        // For now, we'll just create the item

        Ok(())
    }

    /// Mark a todo as done in GitHub Project
    pub fn mark_todo_done(&self, _worktree_name: &str) -> Result<()> {
        // TODO: Implement marking todo as done
        // This would require:
        // 1. Finding the item by worktree name
        // 2. Getting the Status field ID
        // 3. Getting the "Done" option ID
        // 4. Updating the item's status field
        Ok(())
    }

    /// Sync todos from GitHub to local cache
    pub fn sync_from_github(&self) -> Result<Vec<Todo>> {
        self.fetch_todos()
    }

    /// Sync todos from local cache to GitHub
    #[allow(dead_code)]
    pub fn sync_to_github(&self, _todos: &[Todo]) -> Result<()> {
        // TODO: Implement bidirectional sync
        // This would require:
        // 1. Comparing local vs remote todos
        // 2. Creating new items for local todos not in GitHub
        // 3. Updating status for changed items
        // 4. Optionally deleting items removed locally
        Ok(())
    }
}

/// Get current repository owner and name from git remote
pub fn get_repo_info() -> Result<(String, String)> {
    let output = Command::new("gh")
        .args(&["repo", "view", "--json", "owner,name"])
        .output()
        .context("Failed to get repository info")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(anyhow!("Failed to get repository info: {}", stderr));
    }

    let response: serde_json::Value = serde_json::from_slice(&output.stdout)
        .context("Failed to parse repository info")?;

    let owner = response["owner"]["login"]
        .as_str()
        .context("Missing owner")?
        .to_string();

    let name = response["name"]
        .as_str()
        .context("Missing repo name")?
        .to_string();

    Ok((owner, name))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_github_client_creation() {
        let client = GitHubClient::new(
            "owner".to_string(),
            "repo".to_string(),
            1,
        );
        assert_eq!(client.owner, "owner");
        assert_eq!(client.repo, "repo");
        assert_eq!(client.project_number, 1);
    }

    #[test]
    fn test_todo_status_parsing() {
        let todo = Todo {
            description: "Test".to_string(),
            status: TodoStatus::Pending,
            worktree: Some("test-worktree".to_string()),
        };
        assert_eq!(todo.status, TodoStatus::Pending);
    }
}
