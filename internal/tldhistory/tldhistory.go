// Package tldhistory は root zone の diff 履歴を TLD 単位に集約する。
//
// 既存の store.History は zone 全体の diff を 1 ファイルずつ保存する。
// tldhistory.Index はその履歴を走査し、TLD ごとの変更タイムラインを
// data_dir/tld-history/index.json にキャッシュする。2回目以降は未取り込みの
// diff だけを読むため、Web UI の表示を高速に保てる。
//
// RRSIG の再署名 (category=signature) は TLD の実質的な変更ではないため
// インデックス対象から除外する。
package tldhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yfujii/dns-root-diff/internal/store"
)

const (
	indexVersion = 1
	indexDirName = "tld-history"
	indexName    = "index.json"
)

// ErrInvalidTLD は TLD 名が不正な場合に返される。
var ErrInvalidTLD = errors.New("invalid tld")

// tldPattern は root zone に現れる TLD ラベル (LDH / punycode) を表す。
var tldPattern = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

// Summary は一覧表示用の TLD ごとの集計。
type Summary struct {
	TLD             string         `json:"tld"`
	FirstDetectedAt time.Time      `json:"first_detected_at"`
	LastDetectedAt  time.Time      `json:"last_detected_at"`
	Events          int            `json:"events"`
	Changes         int            `json:"changes"`
	ByCategory      map[string]int `json:"by_category"`
}

// Event は 1 回の diff 検知のうち、該当 TLD に関係する変更だけをまとめたもの。
type Event struct {
	ID         string             `json:"id"`
	DetectedAt time.Time          `json:"detected_at"`
	OldSerial  string             `json:"old_serial"`
	NewSerial  string             `json:"new_serial"`
	Summary    store.Summary      `json:"summary"`
	Changes    []store.ChangeJSON `json:"changes"`
}

// Detail は 1 つの TLD の全イベント (新しい順)。
type Detail struct {
	TLD    string  `json:"tld"`
	Events []Event `json:"events"`
}

// indexFile は永続インデックスの JSON 表現。
type indexFile struct {
	Version    int                `json:"version"`
	LastDiffID string             `json:"last_diff_id"`
	DiffCount  int                `json:"diff_count"`
	TLDs       map[string][]Event `json:"tlds"`
}

// Index は root zone の diff 履歴から TLD 別タイムラインを構築する。
type Index struct {
	diffsDir  string
	indexPath string

	mu         sync.Mutex
	loaded     bool
	tlds       map[string][]Event
	lastDiffID string
	diffCount  int
	// parsed はテストで増分読み込みを検証するためのカウンタ。
	parsed int
}

// NewIndex は dataDir 配下の diffs/ と tld-history/index.json を使う Index を生成する。
func NewIndex(dataDir string) *Index {
	return &Index{
		diffsDir:  filepath.Join(dataDir, "diffs"),
		indexPath: filepath.Join(dataDir, indexDirName, indexName),
		tlds:      make(map[string][]Event),
	}
}

// Refresh は未取り込みの diff をインデックスへ反映する。
// 永続インデックスが無い・壊れている・履歴ファイルが減った場合は全件再構築する。
func (i *Index) Refresh() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.refreshLocked()
}

// List は変更履歴を持つ TLD の集計を、最終変更が新しい順で返す。
func (i *Index) List() ([]Summary, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.refreshLocked(); err != nil {
		return nil, err
	}

	out := make([]Summary, 0, len(i.tlds))
	for tld, events := range i.tlds {
		if len(events) == 0 {
			continue
		}
		s := Summary{
			TLD:        tld,
			Events:     len(events),
			ByCategory: map[string]int{},
		}
		for _, ev := range events {
			s.Changes += ev.Summary.Total
			for cat, n := range ev.Summary.ByCategory {
				s.ByCategory[cat] += n
			}
			if s.FirstDetectedAt.IsZero() || ev.DetectedAt.Before(s.FirstDetectedAt) {
				s.FirstDetectedAt = ev.DetectedAt
			}
			if ev.DetectedAt.After(s.LastDetectedAt) {
				s.LastDetectedAt = ev.DetectedAt
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool {
		if !out[a].LastDetectedAt.Equal(out[b].LastDetectedAt) {
			return out[a].LastDetectedAt.After(out[b].LastDetectedAt)
		}
		return out[a].TLD < out[b].TLD
	})
	return out, nil
}

// Get は TLD の全イベントを新しい順で返す。履歴が無い TLD は空の Detail を返す。
func (i *Index) Get(tld string) (Detail, error) {
	normalized, err := NormalizeTLD(tld)
	if err != nil {
		return Detail{}, err
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.refreshLocked(); err != nil {
		return Detail{}, err
	}

	events := i.tlds[normalized]
	out := make([]Event, 0, len(events))
	for n := len(events) - 1; n >= 0; n-- {
		out = append(out, cloneEvent(&events[n]))
	}
	return Detail{TLD: normalized, Events: out}, nil
}

// NormalizeTLD は入力された TLD 名を小文字・末尾ドット無しに正規化する。
// 空白や先頭のドットは許容するが、複数ラベルや不正な文字は拒否する。
func NormalizeTLD(raw string) (string, error) {
	tld := strings.ToLower(strings.TrimSpace(raw))
	tld = strings.Trim(tld, ".")
	if !tldPattern.MatchString(tld) {
		return "", fmt.Errorf("%w: %q", ErrInvalidTLD, raw)
	}
	return tld, nil
}

// TLDOf は root zone のレコード名から TLD ラベルを取り出す。
// 例: "example." -> "example"、"ns1.example." -> "example"、"." は対象外。
func TLDOf(name string) (string, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, ".")
	if n == "" {
		return "", false
	}
	if pos := strings.LastIndexByte(n, '.'); pos >= 0 {
		n = n[pos+1:]
	}
	if !tldPattern.MatchString(n) {
		return "", false
	}
	return n, true
}

func (i *Index) refreshLocked() error {
	ids, err := i.diffIDs()
	if err != nil {
		return err
	}
	if !i.loaded {
		if !i.loadIndexLocked() {
			i.resetLocked()
		}
		i.loaded = true
	}

	start := i.diffCount
	dirty := false
	if i.needsFullRebuild(ids) {
		i.resetLocked()
		start = 0
		dirty = true
	}
	if start < 0 || start > len(ids) {
		start = 0
	}

	for _, id := range ids[start:] {
		i.parsed++
		dirty = true
		entry, err := i.readEntry(id)
		if err != nil {
			// 壊れた履歴 1 件で Web UI 全体を止めない。次回以降もスキップする。
			continue
		}
		i.addEntry(id, &entry)
	}
	if len(ids) > 0 {
		i.lastDiffID = ids[len(ids)-1]
	}
	i.diffCount = len(ids)
	if !dirty {
		return nil
	}
	// 書き込みに失敗してもメモリ上のインデックスは有効なのでエラーにしない。
	_ = i.writeIndexLocked()
	return nil
}

func (i *Index) needsFullRebuild(ids []string) bool {
	if !i.loaded {
		return true
	}
	if i.lastDiffID == "" {
		return len(ids) > 0
	}
	if i.diffCount > len(ids) {
		return true
	}
	pos := sort.SearchStrings(ids, i.lastDiffID)
	if pos == len(ids) || ids[pos] != i.lastDiffID {
		return true
	}
	// インデックス構築時のファイル数と、last_diff_id の位置が一致しない。
	if pos+1 != i.diffCount {
		return true
	}
	return false
}

func (i *Index) resetLocked() {
	i.tlds = make(map[string][]Event)
	i.lastDiffID = ""
	i.diffCount = 0
}

func (i *Index) addEntry(id string, entry *store.Entry) {
	byTLD := make(map[string][]store.ChangeJSON)
	for _, c := range entry.Changes {
		// RRSIG の再署名は TLD の実質的な変更ではない。
		if c.Category == "signature" {
			continue
		}
		tld, ok := TLDOf(c.Name)
		if !ok {
			continue
		}
		byTLD[tld] = append(byTLD[tld], c)
	}
	for tld, changes := range byTLD {
		ev := Event{
			ID:         id,
			DetectedAt: entry.DetectedAt,
			OldSerial:  entry.OldSerial,
			NewSerial:  entry.NewSerial,
			Summary:    summarize(changes),
			Changes:    changes,
		}
		i.tlds[tld] = append(i.tlds[tld], ev)
	}
}

func summarize(changes []store.ChangeJSON) store.Summary {
	s := store.Summary{Total: len(changes), ByCategory: make(map[string]int, len(changes))}
	for _, c := range changes {
		switch c.Kind {
		case "added":
			s.Added++
		case "removed":
			s.Removed++
		case "modified":
			s.Modified++
		}
		s.ByCategory[c.Category]++
	}
	return s
}

func (i *Index) diffIDs() ([]string, error) {
	files, err := os.ReadDir(i.diffsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read diff dir: %w", err)
	}
	ids := make([]string, 0, len(files))
	for _, f := range files {
		id, ok := strings.CutSuffix(f.Name(), ".json")
		if f.IsDir() || !ok || !store.IsValidID(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (i *Index) readEntry(id string) (store.Entry, error) {
	data, err := os.ReadFile(filepath.Join(i.diffsDir, id+".json"))
	if err != nil {
		return store.Entry{}, err
	}
	var e store.Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return store.Entry{}, err
	}
	return e, nil
}

func (i *Index) loadIndexLocked() bool {
	data, err := os.ReadFile(i.indexPath)
	if err != nil {
		return false
	}
	var f indexFile
	if err := json.Unmarshal(data, &f); err != nil {
		return false
	}
	if f.Version != indexVersion || f.TLDs == nil {
		return false
	}
	if f.LastDiffID != "" && !store.IsValidID(f.LastDiffID) {
		return false
	}
	if f.DiffCount < 0 {
		return false
	}
	for tld, events := range f.TLDs {
		if _, err := NormalizeTLD(tld); err != nil {
			return false
		}
		// 永続ファイルは昇順で保存しているが、念のため並べ直す。
		sort.Slice(events, func(a, b int) bool { return events[a].ID < events[b].ID })
	}
	i.tlds = f.TLDs
	i.lastDiffID = f.LastDiffID
	i.diffCount = f.DiffCount
	return true
}

func (i *Index) writeIndexLocked() error {
	if err := os.MkdirAll(filepath.Dir(i.indexPath), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(indexFile{
		Version:    indexVersion,
		LastDiffID: i.lastDiffID,
		DiffCount:  i.diffCount,
		TLDs:       i.tlds,
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(i.indexPath), ".tmp-index-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, i.indexPath)
}

func cloneEvent(in *Event) Event {
	out := *in
	if in.Changes != nil {
		out.Changes = append([]store.ChangeJSON(nil), in.Changes...)
	}
	if in.Summary.ByCategory != nil {
		m := make(map[string]int, len(in.Summary.ByCategory))
		for k, v := range in.Summary.ByCategory {
			m[k] = v
		}
		out.Summary.ByCategory = m
	}
	return out
}
