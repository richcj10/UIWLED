// Package config parses the addon options file (/data/options.json under HA)
// into typed structs used by the rest of the daemon.
//
// The schema mirrors the Home Assistant addon config.yaml schema; keep the
// two in sync when adding fields.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// Switch describes one UniFi switch UIWLED should drive.
// Either Password or SSHKey must be provided.
type Switch struct {
	Name     string `json:"name"`             // display name, used in HA entity ids and the web UI
	Host     string `json:"host"`             // switch IP or hostname (port 22 assumed)
	User     string `json:"user"`             // UniFi site-wide SSH username
	Password string `json:"password,omitempty"`
	SSHKey   string `json:"ssh_key,omitempty"` // PEM-encoded private key
}

// Config is the full addon configuration as read from /data/options.json.
type Config struct {
	Switches   []Switch `json:"switches"`
	ListenPort int      `json:"listen_port"` // HTTP port for UI + REST
	LogLevel   string   `json:"log_level"`   // debug|info|warn|error
	TargetFPS  int      `json:"target_fps"`  // engine tick rate; 5–20 recommended
}

// Load reads and parses the options file. Missing / zero fields are
// backfilled with sensible defaults so a partially specified file still boots.
func Load(path string) (*Config, error) {
	if path == "" {
		path = "/data/options.json"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	c := &Config{ListenPort: 8080, TargetFPS: 15, LogLevel: "info"}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.TargetFPS <= 0 {
		c.TargetFPS = 15
	}
	if c.ListenPort <= 0 {
		c.ListenPort = 8080
	}
	return c, nil
}

// ApplyLogLevel reconfigures the process-wide slog logger to filter at the
// requested level. Called after Load so the log level comes from user config.
// Unknown levels fall back to info.
func ApplyLogLevel(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}
