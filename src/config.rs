use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TmuxWindow {
    pub name: String,
    pub command: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    #[serde(default = "default_windows")]
    pub windows: Vec<TmuxWindow>,
}

fn default_windows() -> Vec<TmuxWindow> {
    vec![
        TmuxWindow {
            name: "rails".to_string(),
            command: Some("bin/rails s".to_string()),
        },
        TmuxWindow {
            name: "tailwind".to_string(),
            command: Some("bin/rails tailwind:watch".to_string()),
        },
        TmuxWindow {
            name: "omnara".to_string(),
            command: Some("omnara --dangerously-skip-permissions".to_string()),
        },
        TmuxWindow {
            name: "shell".to_string(),
            command: None,
        },
    ]
}

impl Default for Config {
    fn default() -> Self {
        Self {
            windows: default_windows(),
        }
    }
}

impl Config {
    /// Load config from default location or create default
    pub fn load() -> Result<Self> {
        let config_path = Self::config_path()?;

        if config_path.exists() {
            let contents = fs::read_to_string(&config_path)
                .context("Failed to read config file")?;
            toml::from_str(&contents).context("Failed to parse config file")
        } else {
            // Create default config
            let config = Self::default();
            config.save()?;
            Ok(config)
        }
    }

    /// Load config from a specific path (useful for testing)
    #[allow(dead_code)]
    pub fn load_from_path(path: &PathBuf) -> Result<Self> {
        if path.exists() {
            let contents = fs::read_to_string(path)
                .context("Failed to read config file")?;
            toml::from_str(&contents).context("Failed to parse config file")
        } else {
            Err(anyhow::anyhow!("Config file does not exist"))
        }
    }

    /// Save config to a specific path (useful for testing)
    #[allow(dead_code)]
    pub fn save_to_path(&self, path: &PathBuf) -> Result<()> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).context("Failed to create config directory")?;
        }

        let contents = toml::to_string_pretty(self)?;
        fs::write(path, contents).context("Failed to write config file")?;

        Ok(())
    }

    /// Save config to default location
    pub fn save(&self) -> Result<()> {
        let config_path = Self::config_path()?;

        if let Some(parent) = config_path.parent() {
            fs::create_dir_all(parent).context("Failed to create config directory")?;
        }

        let contents = toml::to_string_pretty(self)?;
        fs::write(&config_path, contents).context("Failed to write config file")?;

        Ok(())
    }

    /// Get the config file path
    pub fn config_path() -> Result<PathBuf> {
        let config_dir = dirs::config_dir()
            .context("Failed to get config directory")?
            .join("lfg");

        Ok(config_dir.join("config.toml"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    #[test]
    fn test_default_config() {
        let config = Config::default();
        assert_eq!(config.windows.len(), 4);
        assert_eq!(config.windows[0].name, "rails");
        assert_eq!(config.windows[1].name, "tailwind");
        assert_eq!(config.windows[2].name, "omnara");
        assert_eq!(config.windows[3].name, "shell");
    }

    #[test]
    fn test_default_windows() {
        let windows = default_windows();
        assert_eq!(windows.len(), 4);

        assert_eq!(windows[0].name, "rails");
        assert_eq!(windows[0].command, Some("bin/rails s".to_string()));

        assert_eq!(windows[1].name, "tailwind");
        assert_eq!(windows[1].command, Some("bin/rails tailwind:watch".to_string()));

        assert_eq!(windows[2].name, "omnara");
        assert_eq!(windows[2].command, Some("omnara --dangerously-skip-permissions".to_string()));

        assert_eq!(windows[3].name, "shell");
        assert_eq!(windows[3].command, None);
    }

    #[test]
    fn test_tmux_window_with_command() {
        let window = TmuxWindow {
            name: "test".to_string(),
            command: Some("echo hello".to_string()),
        };

        assert_eq!(window.name, "test");
        assert_eq!(window.command, Some("echo hello".to_string()));
    }

    #[test]
    fn test_tmux_window_without_command() {
        let window = TmuxWindow {
            name: "shell".to_string(),
            command: None,
        };

        assert_eq!(window.name, "shell");
        assert_eq!(window.command, None);
    }

    #[test]
    fn test_config_serialization() {
        let config = Config {
            windows: vec![
                TmuxWindow {
                    name: "editor".to_string(),
                    command: Some("nvim".to_string()),
                },
                TmuxWindow {
                    name: "shell".to_string(),
                    command: None,
                },
            ],
        };

        let toml_str = toml::to_string(&config).unwrap();
        assert!(toml_str.contains("editor"));
        assert!(toml_str.contains("nvim"));
        assert!(toml_str.contains("shell"));
    }

    #[test]
    fn test_config_deserialization() {
        let toml_str = r#"
[[windows]]
name = "editor"
command = "nvim"

[[windows]]
name = "shell"
"#;

        let config: Config = toml::from_str(toml_str).unwrap();
        assert_eq!(config.windows.len(), 2);
        assert_eq!(config.windows[0].name, "editor");
        assert_eq!(config.windows[0].command, Some("nvim".to_string()));
        assert_eq!(config.windows[1].name, "shell");
        assert_eq!(config.windows[1].command, None);
    }

    #[test]
    fn test_config_deserialization_empty_windows() {
        let toml_str = r#"
windows = []
"#;

        let config: Config = toml::from_str(toml_str).unwrap();
        assert_eq!(config.windows.len(), 0);
    }

    #[test]
    fn test_config_deserialization_with_default() {
        let toml_str = r#""#;

        let config: Config = toml::from_str(toml_str).unwrap();
        // Should use default_windows
        assert_eq!(config.windows.len(), 4);
    }

    #[test]
    fn test_config_save_and_load() {
        let temp_dir = std::env::temp_dir();
        let test_config_path = temp_dir.join("lfg_test_config.toml");

        // Clean up if exists
        let _ = fs::remove_file(&test_config_path);

        let original_config = Config {
            windows: vec![
                TmuxWindow {
                    name: "test1".to_string(),
                    command: Some("cmd1".to_string()),
                },
                TmuxWindow {
                    name: "test2".to_string(),
                    command: None,
                },
            ],
        };

        // Save config
        original_config.save_to_path(&test_config_path).unwrap();
        assert!(test_config_path.exists());

        // Load config
        let loaded_config = Config::load_from_path(&test_config_path).unwrap();
        assert_eq!(loaded_config.windows.len(), 2);
        assert_eq!(loaded_config.windows[0].name, "test1");
        assert_eq!(loaded_config.windows[0].command, Some("cmd1".to_string()));
        assert_eq!(loaded_config.windows[1].name, "test2");
        assert_eq!(loaded_config.windows[1].command, None);

        // Clean up
        let _ = fs::remove_file(&test_config_path);
    }

    #[test]
    fn test_config_load_nonexistent_file() {
        let temp_dir = std::env::temp_dir();
        let nonexistent_path = temp_dir.join("lfg_nonexistent_12345.toml");

        // Make sure it doesn't exist
        let _ = fs::remove_file(&nonexistent_path);

        let result = Config::load_from_path(&nonexistent_path);
        assert!(result.is_err());
    }

    #[test]
    fn test_config_load_invalid_toml() {
        let temp_dir = std::env::temp_dir();
        let invalid_config_path = temp_dir.join("lfg_invalid_config.toml");

        // Write invalid TOML
        let mut file = fs::File::create(&invalid_config_path).unwrap();
        file.write_all(b"this is not valid TOML {[}]").unwrap();

        let result = Config::load_from_path(&invalid_config_path);
        assert!(result.is_err());

        // Clean up
        let _ = fs::remove_file(&invalid_config_path);
    }

    #[test]
    fn test_config_save_creates_directory() {
        let temp_dir = std::env::temp_dir();
        let nested_dir = temp_dir.join("lfg_test_nested_dir");
        let config_path = nested_dir.join("config.toml");

        // Clean up if exists
        let _ = fs::remove_dir_all(&nested_dir);
        assert!(!nested_dir.exists());

        let config = Config::default();
        config.save_to_path(&config_path).unwrap();

        assert!(nested_dir.exists());
        assert!(config_path.exists());

        // Clean up
        let _ = fs::remove_dir_all(&nested_dir);
    }

    #[test]
    fn test_tmux_window_clone() {
        let window = TmuxWindow {
            name: "test".to_string(),
            command: Some("echo test".to_string()),
        };

        let cloned = window.clone();
        assert_eq!(cloned.name, "test");
        assert_eq!(cloned.command, Some("echo test".to_string()));
    }

    #[test]
    fn test_config_clone() {
        let config = Config {
            windows: vec![
                TmuxWindow {
                    name: "test".to_string(),
                    command: None,
                },
            ],
        };

        let cloned = config.clone();
        assert_eq!(cloned.windows.len(), 1);
        assert_eq!(cloned.windows[0].name, "test");
    }

    #[test]
    fn test_config_debug() {
        let config = Config::default();
        let debug_str = format!("{:?}", config);
        assert!(debug_str.contains("Config"));
        assert!(debug_str.contains("windows"));
    }
}
