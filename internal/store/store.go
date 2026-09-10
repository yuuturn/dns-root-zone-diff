package store

import (
	"fmt"
	"os"
	"path/filepath"
)

const zoneFileName = "root.zone"

const anchorFileName = "root-anchors.xml"

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

// NewAnchor は root anchors スナップショット用の Store を生成する。
func NewAnchor(dir string) *Store {
	return &Store{
		dir:  dir,
		path: filepath.Join(dir, anchorFileName),
	}
}

// Save はゾーンデータをファイルに保存する。
// 一時ファイルに書いてから rename することで、クラッシュ・ディスクフル時に
// スナップショットが部分書き込みで破損するのを防ぐ (History.Append と同じ方式)。
func (s *Store) Save(data []byte) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close snapshot file: %w", err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod snapshot file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename snapshot file: %w", err)
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
