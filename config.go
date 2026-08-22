package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	RefreshSeconds int            `json:"refresh_seconds"`
	PageSeconds    int            `json:"page_seconds"`
	Servers        []ServerConfig `json:"servers"`
	AgentsDir      string         `json:"agents_dir"`
	WorkflowsFile  string         `json:"workflows_file"`
}

type ServerConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Address      string `json:"address,omitempty"`
	User         string `json:"user,omitempty"`
	IdentityFile string `json:"identity_file,omitempty"`
	ProbePath    string `json:"probe_path,omitempty"`
	Sudo         bool   `json:"sudo,omitempty"`
	URL          string `json:"url,omitempty"`
}

func defaultConfigPath() string {
	if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		return filepath.Join(p, "opsdeck", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opsdeck", "config.json")
}

func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func loadConfig(path string) (Config, error) {
	if path == "" {
		path = defaultConfigPath()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, errors.New("opsdeck config not found: " + path)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.RefreshSeconds < 1 {
		cfg.RefreshSeconds = 3
	}
	if cfg.PageSeconds < 1 {
		cfg.PageSeconds = 20
	}
	cfg.AgentsDir = expandHome(cfg.AgentsDir)
	cfg.WorkflowsFile = expandHome(cfg.WorkflowsFile)
	for i := range cfg.Servers {
		cfg.Servers[i].IdentityFile = expandHome(cfg.Servers[i].IdentityFile)
		if cfg.Servers[i].ProbePath == "" {
			cfg.Servers[i].ProbePath = "/usr/local/bin/opsdeck"
		}
	}
	return cfg, nil
}
