package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/age-imap-decryptor/age-imap-decryptor/internal/mail"
)

// noopDecryptor satisfies the *crypto.Decryptor interface via a thin wrapper
// so we can test the processor without loading real age identities.
// We embed a *Processor with a custom decrypt func via a test-only helper.

func newTestProcessor(t *testing.T, ext string) (*Processor, string) {
	t.Helper()
	base := t.TempDir()
	// We can't easily inject a fake Decryptor without changing the production
	// type, so tests that exercise the decrypt path are integration tests.
	// Here we test only the filesystem logic (dedup, filtering, dir naming).
	return &Processor{dec: nil, baseDir: base, ext: ext}, base
}

func TestAlreadyProcessed_False(t *testing.T) {
	proc, _ := newTestProcessor(t, ".age")
	if proc.alreadyProcessed(42) {
		t.Error("fresh dir: alreadyProcessed should return false")
	}
}

func TestAlreadyProcessed_True(t *testing.T) {
	proc, base := newTestProcessor(t, ".age")
	// Create a directory with the expected suffix.
	if err := os.Mkdir(filepath.Join(base, "2026-01-01_00.00.00Z_42"), 0o750); err != nil {
		t.Fatal(err)
	}
	if !proc.alreadyProcessed(42) {
		t.Error("existing dir: alreadyProcessed should return true")
	}
}

func TestAlreadyProcessed_DifferentUID(t *testing.T) {
	proc, base := newTestProcessor(t, ".age")
	if err := os.Mkdir(filepath.Join(base, "2026-01-01_00.00.00Z_99"), 0o750); err != nil {
		t.Fatal(err)
	}
	if proc.alreadyProcessed(42) {
		t.Error("dir for uid 99 should not match uid 42")
	}
}

func TestProcess_SkipsNonAgeAttachment(t *testing.T) {
	proc, base := newTestProcessor(t, ".age")
	msg := &mail.Message{
		UID: 1,
		Attachments: []mail.Attachment{
			{Name: "readme.txt", Content: []byte("not encrypted")},
		},
	}
	// dec is nil; if filtering works, Decrypt is never called, so no panic.
	if err := proc.Process(msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No output directory should have been created.
	entries, _ := os.ReadDir(base)
	if len(entries) != 0 {
		t.Errorf("expected no output dirs for non-.age attachment, got %d", len(entries))
	}
}

func TestProcess_SkipsNoAttachments(t *testing.T) {
	proc, base := newTestProcessor(t, ".age")
	msg := &mail.Message{UID: 7, Attachments: nil}
	if err := proc.Process(msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 0 {
		t.Errorf("expected no output dirs for message with no attachments, got %d", len(entries))
	}
}

func TestProcess_DeduplicatesUID(t *testing.T) {
	proc, base := newTestProcessor(t, ".age")
	// Pre-create a directory that looks like uid 5 was already processed.
	if err := os.Mkdir(filepath.Join(base, "2026-01-01_00.00.00Z_5"), 0o750); err != nil {
		t.Fatal(err)
	}
	msg := &mail.Message{
		UID: 5,
		Attachments: []mail.Attachment{
			{Name: "secret.txt.age", Content: []byte("encrypted")},
		},
	}
	// dec is nil; if dedup works, Decrypt is never called, so no panic.
	if err := proc.Process(msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the pre-existing directory should exist (no new one).
	entries, _ := os.ReadDir(base)
	if len(entries) != 1 {
		t.Errorf("dedup failed: expected 1 dir, got %d", len(entries))
	}
}

func TestRenameByMeta_RenamesPayload(t *testing.T) {
	dir := t.TempDir()
	meta := `{"filename":"screenshot-age-web.png","mime":"image/png","size":1,"sha256":"abc"}`
	if err := os.WriteFile(filepath.Join(dir, "attachment-001.meta.age.plain"), []byte(meta), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attachment-001.payload.age.plain"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	renameByMeta(dir)

	if _, err := os.Stat(filepath.Join(dir, "screenshot-age-web.png")); err != nil {
		t.Errorf("renamed file not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "attachment-001.payload.age.plain")); err == nil {
		t.Error("original payload file should have been renamed away")
	}
}

func TestRenameByMeta_NoPayload(t *testing.T) {
	dir := t.TempDir()
	meta := `{"filename":"output.bin"}`
	if err := os.WriteFile(filepath.Join(dir, "attachment-001.meta.age.plain"), []byte(meta), 0o640); err != nil {
		t.Fatal(err)
	}
	// No payload file — renameByMeta must not panic or error.
	renameByMeta(dir)
}

func TestRenameByMeta_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	meta := `{"filename":"../../evil.txt"}`
	if err := os.WriteFile(filepath.Join(dir, "attachment-001.meta.age.plain"), []byte(meta), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attachment-001.payload.age.plain"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	renameByMeta(dir)
	// The file must land inside dir, not two levels up.
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err != nil {
		t.Error("path-traversal payload should land as evil.txt inside the output dir")
	}
}

func TestCreateOutputDir_NamingConvention(t *testing.T) {
	proc, _ := newTestProcessor(t, ".age")
	dir, err := proc.createOutputDir(123)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(dir)
	if !strings.HasSuffix(name, "_123") {
		t.Errorf("output dir %q should end with _123", name)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("output dir %q was not created", dir)
	}
}
