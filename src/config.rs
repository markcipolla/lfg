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
