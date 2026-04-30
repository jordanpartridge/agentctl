package review

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds connection settings for calling Lexi.
type Config struct {
	LexiURL   string `json:"lexi_url"`
	LexiToken string `json:"lexi_token"`
}

// configPath returns the path to ~/.agentctl/config.json.
func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agentctl", "config.json")
}

// LoadConfig reads ~/.agentctl/config.json, falling back to APP_KEY env for the token.
func LoadConfig() Config {
	cfg := Config{
		LexiURL: "http://localhost:8002",
	}

	data, err := os.ReadFile(configPath())
	if err == nil {
		json.Unmarshal(data, &cfg)
	}

	if cfg.LexiToken == "" {
		cfg.LexiToken = os.Getenv("APP_KEY")
	}

	return cfg
}
