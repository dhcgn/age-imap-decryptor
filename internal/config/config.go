package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type IMAP struct {
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Mailbox  string `yaml:"mailbox"`
}

type Filter struct {
	AttachmentExtension string `yaml:"attachment_extension"`
	Sender              string `yaml:"sender"`
}

type Keys struct {
	Path string `yaml:"path"`
}

type Output struct {
	BaseDir string `yaml:"base_dir"`
}

type Runtime struct {
	InitialScan bool `yaml:"initial_scan"`
	Idle        bool `yaml:"idle"`
}

type Config struct {
	IMAP    IMAP    `yaml:"imap"`
	Filter  Filter  `yaml:"filter"`
	Keys    Keys    `yaml:"keys"`
	Output  Output  `yaml:"output"`
	Runtime Runtime `yaml:"runtime"`
}

func Load(path string) (*Config, error) {
	// Pre-fill defaults so fields absent from YAML retain them.
	cfg := Config{
		IMAP:    IMAP{Port: 993, Mailbox: "INBOX"},
		Filter:  Filter{AttachmentExtension: ".age"},
		Keys:    Keys{Path: defaultKeysPath()},
		Runtime: Runtime{InitialScan: true, Idle: true},
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.Keys.Path = expandPath(cfg.Keys.Path)
	cfg.Output.BaseDir = expandPath(cfg.Output.BaseDir)

	if pw := os.Getenv("WATCHER_IMAP_PASSWORD"); pw != "" {
		cfg.IMAP.Password = pw
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// expandPath expands a leading ~/ to the user's home directory.
func expandPath(s string) string {
	if !strings.HasPrefix(s, "~/") {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	return filepath.Join(home, s[2:])
}

func defaultKeysPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".age"
	}
	return filepath.Join(home, ".age")
}

func validate(cfg *Config) error {
	if cfg.IMAP.Server == "" {
		return errors.New("imap.server is required")
	}
	if cfg.IMAP.Username == "" {
		return errors.New("imap.username is required")
	}
	if cfg.IMAP.Password == "" {
		return errors.New("imap.password is required (or set WATCHER_IMAP_PASSWORD)")
	}
	if cfg.Keys.Path == "" {
		return errors.New("keys.path is required")
	}
	if cfg.Output.BaseDir == "" {
		return errors.New("output.base_dir is required")
	}
	if !cfg.Runtime.InitialScan && !cfg.Runtime.Idle {
		return errors.New("at least one of runtime.initial_scan or runtime.idle must be true")
	}
	return nil
}
