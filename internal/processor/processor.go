package processor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/age-imap-decryptor/age-imap-decryptor/internal/crypto"
	"github.com/age-imap-decryptor/age-imap-decryptor/internal/mail"
)

// Processor decrypts .age attachments from messages and writes the plaintext
// to the output directory.
type Processor struct {
	dec     *crypto.Decryptor
	baseDir string
	ext     string
}

// New creates a Processor that writes to baseDir and filters attachments by ext.
func New(dec *crypto.Decryptor, baseDir string, ext string) *Processor {
	return &Processor{dec: dec, baseDir: baseDir, ext: ext}
}

// Process decrypts all matching attachments of msg into a new output directory.
// It is a no-op when the message was already processed (dedup via dir suffix).
// A write failure is treated as fatal and returned immediately.
func (p *Processor) Process(msg *mail.Message) error {
	if p.alreadyProcessed(msg.UID) {
		return nil
	}

	var targets []mail.Attachment
	for _, a := range msg.Attachments {
		if strings.HasSuffix(a.Name, p.ext) {
			targets = append(targets, a)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	dir, err := p.createOutputDir(msg.UID)
	if err != nil {
		return err
	}

	for _, a := range targets {
		plain, err := p.dec.Decrypt(a.Content)
		if err != nil {
			return fmt.Errorf("decrypt attachment %q (uid %d): %w", a.Name, msg.UID, err)
		}
		outPath := filepath.Join(dir, a.Name+".plain")
		if err := os.WriteFile(outPath, plain, 0o640); err != nil {
			return fmt.Errorf("write %q: %w", outPath, err)
		}
	}
	renameByMeta(dir)
	return nil
}

const (
	metaSuffix    = ".meta.age.plain"
	payloadSuffix = ".payload.age.plain"
)

type metaSidecar struct {
	Filename string `json:"filename"`
}

// renameByMeta scans dir for *.meta.age.plain sidecar files. For each one it
// finds the matching *.payload.age.plain file and renames it to the filename
// field declared in the sidecar JSON. Errors are non-fatal: a bad sidecar
// is skipped so that the rest of the output is not affected.
func renameByMeta(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), metaSuffix) {
			continue
		}

		prefix := strings.TrimSuffix(e.Name(), metaSuffix)
		payloadPath := filepath.Join(dir, prefix+payloadSuffix)
		if _, err := os.Stat(payloadPath); err != nil {
			continue // no matching payload
		}

		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta metaSidecar
		if err := json.Unmarshal(raw, &meta); err != nil || meta.Filename == "" {
			continue
		}

		// Sanitise: strip any directory components to prevent path traversal.
		target := filepath.Join(dir, filepath.Base(meta.Filename))
		_ = os.Rename(payloadPath, target)
	}
}

// alreadyProcessed returns true if a directory with the _<uid> suffix exists.
func (p *Processor) alreadyProcessed(uid int) bool {
	suffix := fmt.Sprintf("_%d", uid)
	entries, err := os.ReadDir(p.baseDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			return true
		}
	}
	return false
}

// createOutputDir creates and returns the <timestamp>_<uid> directory.
func (p *Processor) createOutputDir(uid int) (string, error) {
	ts := time.Now().UTC().Format("2006-01-02_15.04.05Z")
	dir := filepath.Join(p.baseDir, fmt.Sprintf("%s_%d", ts, uid))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create output dir %q: %w", dir, err)
	}
	return dir, nil
}
