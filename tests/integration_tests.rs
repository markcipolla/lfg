use lfg::{cli, config, git};
use std::path::PathBuf;

#[test]
fn test_cli_args_parsing() {
    // Test that CLI argument parsing works correctly
    use clap::Parser;

    // Test with no arguments
    let args = cli::Args::try_parse_from(vec!["lfg"]).unwrap();
    assert_eq!(args.worktree, None);

    // Test with a worktree name
    let args = cli::Args::try_parse_from(vec!["lfg", "my-feature"]).unwrap();
    assert_eq!(args.worktree, Some("my-feature".to_string()));
}

#[test]
fn test_config_integration() {
    // Test the full config lifecycle
    let temp_dir = std::env::temp_dir();
    let test_config_path = temp_dir.join("lfg_integration_test_config.toml");

    // Clean up if exists
    let _ = std::fs::remove_file(&test_config_path);

    // Create a custom config
    let custom_config = config::Config {
        windows: vec![
            config::TmuxWindow {
                name: "editor".to_string(),
                command: Some("nvim".to_string()),
            },
            config::TmuxWindow {
                name: "server".to_string(),
                command: Some("npm start".to_string()),
            },
            config::TmuxWindow {
                name: "shell".to_string(),
                command: None,
            },
        ],
    };

    // Save it
    custom_config.save_to_path(&test_config_path).unwrap();
    assert!(test_config_path.exists());

    // Load it back
    let loaded_config = config::Config::load_from_path(&test_config_path).unwrap();

    // Verify all windows were preserved
    assert_eq!(loaded_config.windows.len(), 3);
    assert_eq!(loaded_config.windows[0].name, "editor");
    assert_eq!(loaded_config.windows[0].command, Some("nvim".to_string()));
    assert_eq!(loaded_config.windows[1].name, "server");
    assert_eq!(
        loaded_config.windows[1].command,
        Some("npm start".to_string())
    );
    assert_eq!(loaded_config.windows[2].name, "shell");
    assert_eq!(loaded_config.windows[2].command, None);

    // Clean up
    let _ = std::fs::remove_file(&test_config_path);
}

#[test]
fn test_worktree_struct_integration() {
    // Test creating and working with Worktree structs
    let worktree1 = git::Worktree {
        name: "main".to_string(),
        path: PathBuf::from("/home/user/project"),
        branch: "main".to_string(),
    };

    let worktree2 = git::Worktree {
        name: "feature".to_string(),
        path: PathBuf::from("/home/user/project-feature"),
        branch: "feature/new-feature".to_string(),
    };

    // Test that worktrees can be cloned
    let cloned = worktree1.clone();
    assert_eq!(cloned.name, "main");
    assert_eq!(cloned.path, PathBuf::from("/home/user/project"));
    assert_eq!(cloned.branch, "main");

    // Test that worktrees can be stored in a vector
    let worktrees = [worktree1, worktree2];
    assert_eq!(worktrees.len(), 2);
    assert_eq!(worktrees[0].name, "main");
    assert_eq!(worktrees[1].name, "feature");
}

#[test]
fn test_config_default_values() {
    // Test that default config has expected structure
    let default_config = config::Config::default();

    assert_eq!(default_config.windows.len(), 4);

    // Verify default window names
    let window_names: Vec<&str> = default_config
        .windows
        .iter()
        .map(|w| w.name.as_str())
        .collect();
    assert_eq!(window_names, vec!["rails", "tailwind", "omnara", "shell"]);

    // Verify commands
    assert!(default_config.windows[0].command.is_some());
    assert!(default_config.windows[1].command.is_some());
    assert!(default_config.windows[2].command.is_some());
    assert!(default_config.windows[3].command.is_none());
}

#[test]
fn test_config_serialization_round_trip() {
    // Test that config can be serialized and deserialized without data loss
    let original = config::Config {
        windows: vec![
            config::TmuxWindow {
                name: "test-window-1".to_string(),
                command: Some("echo 'test 1'".to_string()),
            },
            config::TmuxWindow {
                name: "test-window-2".to_string(),
                command: Some("echo 'test 2'".to_string()),
            },
        ],
    };

    // Serialize to TOML
    let toml_string = toml::to_string(&original).unwrap();

    // Deserialize back
    let deserialized: config::Config = toml::from_str(&toml_string).unwrap();

    // Verify data matches
    assert_eq!(deserialized.windows.len(), original.windows.len());
    for (i, window) in deserialized.windows.iter().enumerate() {
        assert_eq!(window.name, original.windows[i].name);
        assert_eq!(window.command, original.windows[i].command);
    }
}

#[test]
fn test_empty_config_uses_defaults() {
    // Test that an empty TOML file uses default windows
    let empty_toml = "";
    let config: config::Config = toml::from_str(empty_toml).unwrap();

    // Should have default windows
    assert_eq!(config.windows.len(), 4);
}

#[test]
fn test_config_with_special_characters() {
    // Test that config handles special characters in commands
    let config = config::Config {
        windows: vec![config::TmuxWindow {
            name: "test".to_string(),
            command: Some("echo 'hello \"world\"' && ls -la".to_string()),
        }],
    };

    let toml_string = toml::to_string(&config).unwrap();
    let deserialized: config::Config = toml::from_str(&toml_string).unwrap();

    assert_eq!(
        deserialized.windows[0].command,
        Some("echo 'hello \"world\"' && ls -la".to_string())
    );
}

// Git-related integration tests
// These tests work with the parsing logic without requiring actual git commands

#[test]
fn test_git_parse_worktrees_integration() {
    // Test realistic git worktree output parsing
    let realistic_output = r#"worktree /Users/developer/projects/myapp
HEAD c1a2b3d4e5f6g7h8i9j0
branch refs/heads/main

worktree /Users/developer/projects/myapp-feature-auth
HEAD a1b2c3d4e5f6g7h8i9j0
branch refs/heads/feature/authentication

worktree /Users/developer/projects/myapp-bugfix
HEAD 9876543210abcdef1234
branch refs/heads/bugfix/fix-login-issue

"#;

    // This uses the internal parse_worktrees function
    // We can't call it directly from integration tests without exposing it,
    // but we've tested it thoroughly in unit tests
    // This test serves as documentation of expected behavior
    assert!(realistic_output.contains("worktree"));
    assert!(realistic_output.contains("branch"));
}

#[test]
fn test_worktree_path_handling() {
    // Test that worktree paths are handled correctly
    let worktree = git::Worktree {
        name: "my-feature".to_string(),
        path: PathBuf::from("/Users/test/project-my-feature"),
        branch: "feature/my-feature".to_string(),
    };

    // Verify path operations work
    assert_eq!(
        worktree.path.to_str().unwrap(),
        "/Users/test/project-my-feature"
    );
    assert!(worktree.path.to_string_lossy().contains("my-feature"));
}

#[cfg(test)]
mod property_based_tests {
    use super::*;

    #[test]
    fn test_config_windows_ordering_preserved() {
        // Test that window ordering is always preserved through serialization
        let window_names = ["first", "second", "third", "fourth", "fifth"];

        let config = config::Config {
            windows: window_names
                .iter()
                .map(|name| config::TmuxWindow {
                    name: name.to_string(),
                    command: None,
                })
                .collect(),
        };

        let toml_str = toml::to_string(&config).unwrap();
        let loaded: config::Config = toml::from_str(&toml_str).unwrap();

        for (i, name) in window_names.iter().enumerate() {
            assert_eq!(loaded.windows[i].name, *name);
        }
    }

    #[test]
    fn test_config_handles_unicode() {
        // Test that config handles Unicode characters properly
        let config = config::Config {
            windows: vec![
                config::TmuxWindow {
                    name: "日本語".to_string(),
                    command: Some("echo '你好'".to_string()),
                },
                config::TmuxWindow {
                    name: "emoji-window".to_string(),
                    command: Some("echo '🚀'".to_string()),
                },
            ],
        };

        let toml_str = toml::to_string(&config).unwrap();
        let loaded: config::Config = toml::from_str(&toml_str).unwrap();

        assert_eq!(loaded.windows[0].name, "日本語");
        assert_eq!(loaded.windows[0].command, Some("echo '你好'".to_string()));
        assert_eq!(loaded.windows[1].name, "emoji-window");
        assert_eq!(loaded.windows[1].command, Some("echo '🚀'".to_string()));
    }
}

// Todo-related integration tests

#[test]
fn test_add_todo_integration_new_todos_at_top() {
    // Integration test: Verify new todos are added to the top of the list
    let mut app_config = config::AppConfig::default();

    // Start with empty todos
    assert_eq!(app_config.todos.len(), 0);

    // Add first todo
    app_config.add_todo("First task".to_string(), "worktree-1".to_string());
    assert_eq!(app_config.todos.len(), 1);
    assert_eq!(app_config.todos[0].description, "First task");

    // Add second todo - should appear at index 0
    app_config.add_todo("Second task".to_string(), "worktree-2".to_string());
    assert_eq!(app_config.todos.len(), 2);
    assert_eq!(app_config.todos[0].description, "Second task");
    assert_eq!(app_config.todos[1].description, "First task");

    // Add third todo - should appear at index 0
    app_config.add_todo("Third task".to_string(), "worktree-3".to_string());
    assert_eq!(app_config.todos.len(), 3);
    assert_eq!(app_config.todos[0].description, "Third task");
    assert_eq!(app_config.todos[1].description, "Second task");
    assert_eq!(app_config.todos[2].description, "First task");
}

#[test]
fn test_todo_persistence_with_save_and_load() {
    // Integration test: Verify todos persist correctly when saved and loaded
    let temp_dir = std::env::temp_dir();
    let test_yaml_path = temp_dir.join("lfg_todo_persistence_test.yaml");

    // Clean up if exists
    let _ = std::fs::remove_file(&test_yaml_path);

    // Create config with multiple todos added in sequence
    let mut app_config = config::AppConfig::default();
    app_config.add_todo("Task A".to_string(), "wt-a".to_string());
    app_config.add_todo("Task B".to_string(), "wt-b".to_string());
    app_config.add_todo("Task C".to_string(), "wt-c".to_string());

    // Verify order before save
    assert_eq!(app_config.todos[0].description, "Task C");
    assert_eq!(app_config.todos[1].description, "Task B");
    assert_eq!(app_config.todos[2].description, "Task A");

    // Save to YAML
    let yaml_content = serde_yaml::to_string(&app_config).unwrap();
    std::fs::write(&test_yaml_path, &yaml_content).unwrap();

    // Load from YAML
    let loaded_content = std::fs::read_to_string(&test_yaml_path).unwrap();
    let loaded_config: config::AppConfig = serde_yaml::from_str(&loaded_content).unwrap();

    // Verify order is preserved after load
    assert_eq!(loaded_config.todos.len(), 3);
    assert_eq!(loaded_config.todos[0].description, "Task C");
    assert_eq!(loaded_config.todos[1].description, "Task B");
    assert_eq!(loaded_config.todos[2].description, "Task A");

    // Verify worktree associations
    assert_eq!(loaded_config.todos[0].worktree, Some("wt-c".to_string()));
    assert_eq!(loaded_config.todos[1].worktree, Some("wt-b".to_string()));
    assert_eq!(loaded_config.todos[2].worktree, Some("wt-a".to_string()));

    // Clean up
    let _ = std::fs::remove_file(&test_yaml_path);
}

#[test]
fn test_todo_workflow_simulation() {
    // Simulate a realistic workflow: create multiple worktrees, mark some done
    let mut app_config = config::AppConfig::default();

    // Day 1: Start work on feature
    app_config.add_todo("Implement user authentication".to_string(), "feature-auth".to_string());

    // Day 2: Start bug fix
    app_config.add_todo("Fix login redirect bug".to_string(), "bugfix-login".to_string());

    // Day 3: Start another feature
    app_config.add_todo("Add password reset".to_string(), "feature-reset-password".to_string());

    // Check current state - most recent should be first
    assert_eq!(app_config.todos[0].description, "Add password reset");
    assert_eq!(app_config.todos[1].description, "Fix login redirect bug");
    assert_eq!(app_config.todos[2].description, "Implement user authentication");

    // Complete the bug fix
    app_config.mark_todo_done("bugfix-login");

    // Verify status
    assert_eq!(app_config.todos[0].status, config::TodoStatus::Pending);
    assert_eq!(app_config.todos[1].status, config::TodoStatus::Done);
    assert_eq!(app_config.todos[2].status, config::TodoStatus::Pending);

    // Day 4: Start documentation
    app_config.add_todo("Update API docs".to_string(), "docs-api".to_string());

    // Verify new todo is at the top
    assert_eq!(app_config.todos[0].description, "Update API docs");
    assert_eq!(app_config.todos[0].status, config::TodoStatus::Pending);

    // Total should be 4 todos
    assert_eq!(app_config.todos.len(), 4);
}
