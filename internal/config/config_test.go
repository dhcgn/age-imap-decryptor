package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestDefaults(t *testing.T) {
	p := writeTemp(t, `
imap:
  server: "imap.example.com"
  username: "user@example.com"
  password: "secret"
output:
  base_dir: "/tmp/out"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IMAP.Port != 993 {
		t.Errorf("port default: got %d, want 993", cfg.IMAP.Port)
	}
	if cfg.IMAP.Mailbox != "INBOX" {
		t.Errorf("mailbox default: got %q, want INBOX", cfg.IMAP.Mailbox)
	}
	if cfg.Filter.AttachmentExtension != ".age" {
		t.Errorf("ext default: got %q, want .age", cfg.Filter.AttachmentExtension)
	}
	if !cfg.Runtime.InitialScan {
		t.Error("initial_scan default should be true")
	}
	if !cfg.Runtime.Idle {
		t.Error("idle default should be true")
	}
}

func TestEnvPasswordOverride(t *testing.T) {
	t.Setenv("WATCHER_IMAP_PASSWORD", "env-password")
	p := writeTemp(t, `
imap:
  server: "imap.example.com"
  username: "user@example.com"
  password: "yaml-password"
output:
  base_dir: "/tmp/out"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IMAP.Password != "env-password" {
		t.Errorf("env override: got %q, want env-password", cfg.IMAP.Password)
	}
}

func TestTildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir:", err)
	}
	p := writeTemp(t, `
imap:
  server: "imap.example.com"
  username: "user@example.com"
  password: "secret"
keys:
  path: "~/.age"
output:
  base_dir: "~/decrypted"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := filepath.Join(home, ".age")
	if cfg.Keys.Path != wantKeys {
		t.Errorf("keys path: got %q, want %q", cfg.Keys.Path, wantKeys)
	}
	wantOut := filepath.Join(home, "decrypted")
	if cfg.Output.BaseDir != wantOut {
		t.Errorf("base_dir: got %q, want %q", cfg.Output.BaseDir, wantOut)
	}
}

func TestValidationBothModesOff(t *testing.T) {
	p := writeTemp(t, `
imap:
  server: "imap.example.com"
  username: "user@example.com"
  password: "secret"
output:
  base_dir: "/tmp/out"
runtime:
  initial_scan: false
  idle: false
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error when both modes are false")
	}
	if !strings.Contains(err.Error(), "initial_scan") {
		t.Errorf("error message should mention initial_scan: %v", err)
	}
}

func TestValidationMissingServer(t *testing.T) {
	p := writeTemp(t, `
imap:
  username: "user@example.com"
  password: "secret"
output:
  base_dir: "/tmp/out"
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestExpandPathNoTilde(t *testing.T) {
	in := "/absolute/path"
	if got := expandPath(in); got != in {
		t.Errorf("expandPath(%q) = %q, want unchanged", in, got)
	}
}
