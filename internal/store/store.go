package store

import (
	"fmt"
	"os"
	"path/filepath"
)

const zoneFileName = "root.zone"

// Store はゾーンファイルをローカルディスクに永続化する。
type Store struct {
	dir  string
	path string
}

// New は Store を生成する。
func New(dir string) *Store {
	return &Store{
		dir:  dir,
		path: filepath.Join(dir, zoneFileName),
	}
}

// Save はゾーンデータをファイルに保存する。
func (s *Store) Save(data []byte) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("write zone file: %w", err)
	}
	return nil
}

// Load は保存されたゾーンデータを読み込む。
func (s *Store) Load() ([]byte, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Exists は保存されたゾーンファイルが存在するかを返す。
func (s *Store) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}
