package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	data := []byte(".\t86400\tIN\tNS\ta.root-servers.net.\n")
	if err := s.Save(data); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(loaded) != string(data) {
		t.Errorf("Load() = %q, want %q", string(loaded), string(data))
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	if s.Exists() {
		t.Error("Exists() = true on empty dir")
	}

	if err := s.Save([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if !s.Exists() {
		t.Error("Exists() = false after Save")
	}
}

func TestLoadNotExists(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	_, err := s.Load()
	if err == nil {
		t.Fatal("Load() expected error when file not exists")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Load() error = %v, want os.IsNotExist", err)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	s := New(dir)

	if err := s.Save([]byte("test")); err != nil {
		t.Fatalf("Save() should create parent dirs, error = %v", err)
	}
	if !s.Exists() {
		t.Error("Exists() = false after Save with nested dir")
	}
}
