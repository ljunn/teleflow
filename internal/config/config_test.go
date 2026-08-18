package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSessionKey(t *testing.T) {
	directory := t.TempDir()
	first, err := loadOrCreateSessionKey(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateSessionKey(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || !bytes.Equal(first, second) {
		t.Fatalf("session key was not persisted correctly")
	}
	info, err := os.Stat(filepath.Join(directory, "session.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session key permissions are %o", info.Mode().Perm())
	}
}

func TestLoadOrCreateSessionKeyRejectsInvalidKey(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "session.key"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateSessionKey(directory); err == nil {
		t.Fatal("expected invalid key length to fail")
	}
}
