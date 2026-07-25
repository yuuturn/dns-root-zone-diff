package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

const historyDirName = "diffs"

// ErrInvalidID は履歴エントリの ID が不正な場合に返される。
var ErrInvalidID = errors.New("invalid history entry id")

// idPattern は履歴エントリ ID の形式 (<UTC時刻compact>-<SOAシリアル>)。
// ファイル名に使うため、パストラバーサルを防ぐ目的でも検証する。
var idPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9A-Za-z]+$`)

// Entry は1回の変更検知イベントを表す。
type Entry struct {
	ID         string       `json:"id"`
	DetectedAt time.Time    `json:"detected_at"`
	OldSerial  string       `json:"old_serial"`
	NewSerial  string       `json:"new_serial"`
	Summary    Summary      `json:"summary"`
	Changes    []ChangeJSON `json:"changes"`
}

// Summary は変更件数の集計。
type Summary struct {
	Total      int            `json:"total"`
	Added      int            `json:"added"`
	Removed    int            `json:"removed"`
	Modified   int            `json:"modified"`
	ByCategory map[string]int `json:"by_category"`
}

// ChangeJSON は diff.Change の JSON 表現。
// TTL はその側にレコードが存在する場合のみ持つ (ポインター)。
// TTL 0 は DNS として有効な値のため、omitempty で欠落しないよう
// 「存在しない」は nil で表現する。
type ChangeJSON struct {
	Kind     string  `json:"kind"`
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	OldTTL   *uint32 `json:"old_ttl,omitempty"`
	NewTTL   *uint32 `json:"new_ttl,omitempty"`
	OldRData string  `json:"old_rdata,omitempty"`
	NewRData string  `json:"new_rdata,omitempty"`
}

// NewEntry は検知時刻・新旧シリアル・変更群から Entry を組み立てる。
func NewEntry(detectedAt time.Time, oldSerial, newSerial string, changes []diff.Change) Entry {
	detectedAt = detectedAt.UTC()
	e := Entry{
		ID:         fmt.Sprintf("%s-%s", detectedAt.Format("20060102T150405Z"), newSerial),
		DetectedAt: detectedAt,
		OldSerial:  oldSerial,
		NewSerial:  newSerial,
		Summary: Summary{
			Total:      len(changes),
			ByCategory: make(map[string]int),
		},
		Changes: make([]ChangeJSON, 0, len(changes)),
	}
	for _, c := range changes {
		switch c.Kind {
		case diff.ChangeAdded:
			e.Summary.Added++
		case diff.ChangeRemoved:
			e.Summary.Removed++
		case diff.ChangeModified:
			e.Summary.Modified++
		}
		cat := diff.Categorize(c).String()
		e.Summary.ByCategory[cat]++
		cj := ChangeJSON{
			Kind:     c.Kind.String(),
			Category: cat,
			Name:     c.Name,
			Type:     c.Type,
			OldRData: c.OldRData,
			NewRData: c.NewRData,
		}
		// レコードが存在する側の TTL のみ記録する (added に旧側はない、等)。
		oldTTL, newTTL := c.OldTTL, c.NewTTL
		switch c.Kind {
		case diff.ChangeAdded:
			cj.NewTTL = &newTTL
		case diff.ChangeRemoved:
			cj.OldTTL = &oldTTL
		case diff.ChangeModified:
			cj.OldTTL = &oldTTL
			cj.NewTTL = &newTTL
		}
		e.Changes = append(e.Changes, cj)
	}
	return e
}

// History は diff 履歴を JSON ファイルとして永続化する。
// 1検知イベント = 1ファイルで、ファイル名は "<ID>.json"。
type History struct {
	dir string
}

// NewHistory は dataDir 配下の diffs ディレクトリを使う History を生成する。
func NewHistory(dataDir string) *History {
	return &History{dir: filepath.Join(dataDir, historyDirName)}
}

// Append はエントリを新しいファイルとして保存する。
// 一時ファイルに書いてから rename することで部分書き込みを防ぐ。
func (h *History) Append(e Entry) error {
	if !idPattern.MatchString(e.ID) {
		return fmt.Errorf("%w: %q", ErrInvalidID, e.ID)
	}
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history entry: %w", err)
	}
	tmp, err := os.CreateTemp(h.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write history entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod history entry: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(h.dir, e.ID+".json")); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename history entry: %w", err)
	}
	return nil
}

// List は全エントリを新しい順 (ID の辞書降順 = 時系列降順) で返す。
// ディレクトリが存在しない場合は空を返す。パースできないファイルはスキップする。
func (h *History) List() ([]Entry, error) {
	files, err := os.ReadDir(h.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		id, isJSON := strings.CutSuffix(f.Name(), ".json")
		if f.IsDir() || !isJSON || !idPattern.MatchString(id) {
			continue
		}
		e, err := h.readEntry(id)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID > entries[j].ID
	})
	return entries, nil
}

// Get は ID を指定してエントリを取得する。
// ID 形式が不正な場合は ErrInvalidID、存在しない場合は fs.ErrNotExist をラップして返す。
func (h *History) Get(id string) (Entry, error) {
	if !idPattern.MatchString(id) {
		return Entry{}, fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return h.readEntry(id)
}

func (h *History) readEntry(id string) (Entry, error) {
	data, err := os.ReadFile(filepath.Join(h.dir, id+".json"))
	if err != nil {
		return Entry{}, fmt.Errorf("read history entry %s: %w", id, err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return Entry{}, fmt.Errorf("parse history entry %s: %w", id, err)
	}
	return e, nil
}
