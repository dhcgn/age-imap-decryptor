package crypto

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// Decryptor holds the loaded age identities used for decryption.
type Decryptor struct {
	identities []age.Identity
}

// LoadIdentities reads all age identity files from dir and returns a Decryptor.
func LoadIdentities(dir string) (*Decryptor, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read keys dir %q: %w", dir, err)
	}

	var identities []age.Identity
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open key file %q: %w", path, err)
		}
		ids, parseErr := age.ParseIdentities(f)
		f.Close()
		if parseErr != nil {
			return nil, fmt.Errorf("parse identities from %q: %w", path, parseErr)
		}
		identities = append(identities, ids...)
	}

	if len(identities) == 0 {
		return nil, fmt.Errorf("no age identities found in %q", dir)
	}
	return &Decryptor{identities: identities}, nil
}

// Decrypt decrypts an age-encrypted payload using the loaded identities.
func (d *Decryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(ciphertext), d.identities...)
	if err != nil {
		return nil, fmt.Errorf("age decrypt: %w", err)
	}
	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read decrypted stream: %w", err)
	}
	return plaintext, nil
}
