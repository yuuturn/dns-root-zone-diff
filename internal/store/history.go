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
	"sync"
	"time"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

const historyDirName = "diffs"

const anchorHistoryDirName = "anchor-diffs"

const indexFileName = ".index.json"

// ErrInvalidID は履歴エントリの ID が不正な場合に返される。
var ErrInvalidID = errors.New("invalid history entry id")

// idPattern は履歴エントリ ID の形式 (<UTC時刻compact>-<serial相当>)。
// serial 部分は SOA serial の数字のほか、root anchors の TrustAnchor id のように
// ハイフンを含む文字列も許可する。ファイル名に使うため、パストラバーサルを防ぐ
// 目的でも検証する。
var idPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9A-Za-z-]+$`)

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

	mu         sync.RWMutex
	cached     []Entry
	cachedErr  error
	cachedMod  time.Time
	cachedSize int // キャッシュ時のファイル数（削除検知用）
}

// entryHeader は List 用の軽量デコード構造体。Changes を含まないため
// 1エントリあたり数百Bで済み、大量の changes を持つファイルでも高速。
type entryHeader struct {
	ID         string    `json:"id"`
	DetectedAt time.Time `json:"detected_at"`
	OldSerial  string    `json:"old_serial"`
	NewSerial  string    `json:"new_serial"`
	Summary    Summary   `json:"summary"`
}

// NewHistory は dataDir 配下の diffs ディレクトリを使う History を生成する。
func NewHistory(dataDir string) *History {
	return &History{dir: filepath.Join(dataDir, historyDirName)}
}

// NewAnchorHistory は dataDir 配下の anchor-diffs ディレクトリを使う History を生成する。
// root anchors の差分履歴は zone の差分履歴 (diffs/) とは別に保存する。
func NewAnchorHistory(dataDir string) *History {
	return &History{dir: filepath.Join(dataDir, anchorHistoryDirName)}
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
	// キャッシュを無効化。新規ファイル追加でディレクトリ mtime が変わるが、
	// 念のため明示的にクリアして次回 List で再読込させる。
	h.mu.Lock()
	h.cached = nil
	h.cachedErr = nil
	h.cachedMod = time.Time{}
	h.cachedSize = 0
	h.mu.Unlock()
	// インデックスを非同期で更新せず同期で更新する（失敗しても List はフォールバック可能）。
	_ = h.updateIndex(e)
	return nil
}

// List は全エントリを新しい順 (ID の辞書降順 = 時系列降順) で返す。
// ディレクトリが存在しない場合は空を返す。パースできないファイルはスキップする。
//
// 性能: 一覧 API では Summary のみ必要で Changes (数千件) は不要。
// そのため header のみをデコードし、Changes のパースをスキップする。
// さらに `.index.json` による永続インデックスとインメモリキャッシュで
// 連続リクエストでのファイル I/O を回避する。
func (h *History) List() ([]Entry, error) {
	// キャッシュヒット判定。ディレクトリが存在しない場合はキャッシュしない。
	if entries, err, ok := h.cachedList(); ok {
		return cloneEntries(entries), err
	}

	// 永続インデックスを試す。存在し、ファイル数と一致すればそれを使う。
	if entries, ok := h.readIndexIfValid(); ok {
		h.storeCache(entries, nil, nil)
		return cloneEntries(entries), nil
	}

	files, err := os.ReadDir(h.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	// 有効な ID のファイル名のみ抽出し、ソートしてから読み込む。
	// ファイル名自体が ID なので辞書降順 = 時系列降順。
	ids := make([]string, 0, len(files))
	for _, f := range files {
		id, isJSON := strings.CutSuffix(f.Name(), ".json")
		if f.IsDir() || !isJSON || !idPattern.MatchString(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		e, err := h.readEntryHeader(id)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}

	h.storeCache(entries, nil, files)
	// インデックスが無かった場合は再構築しておく（次回から高速）。
	if len(entries) > 0 {
		_ = h.writeIndex(entries)
	}
	return cloneEntries(entries), nil
}

func (h *History) cachedList() ([]Entry, error, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cached == nil && h.cachedErr == nil {
		return nil, nil, false
	}
	info, err := os.Stat(h.dir)
	if err != nil {
		return nil, nil, false
	}
	mod := info.ModTime()
	if !mod.Equal(h.cachedMod) {
		return nil, nil, false
	}
	// mtime が同じならヒットとみなす（Append で明示的 invalidate するので十分）。
	return h.cached, h.cachedErr, true
}

func (h *History) storeCache(entries []Entry, cachedErr error, files []os.DirEntry) {
	info, err := os.Stat(h.dir)
	if err != nil {
		return
	}
	validCount := len(entries)
	if files != nil {
		// 可能なら正確なファイル数を数えるが、なくても entries 長さで代替。
		c := 0
		for _, f := range files {
			if id, ok := strings.CutSuffix(f.Name(), ".json"); ok && !f.IsDir() && idPattern.MatchString(id) {
				c++
			}
		}
		if c != 0 {
			validCount = c
		}
	}
	h.mu.Lock()
	h.cached = cloneEntries(entries)
	h.cachedErr = cachedErr
	h.cachedMod = info.ModTime()
	h.cachedSize = validCount
	h.mu.Unlock()
}

func (h *History) indexPath() string { return filepath.Join(h.dir, indexFileName) }

func (h *History) readIndexIfValid() ([]Entry, bool) {
	data, err := os.ReadFile(h.indexPath())
	if err != nil {
		return nil, false
	}
	var headers []entryHeader
	if err := json.Unmarshal(data, &headers); err != nil {
		return nil, false
	}
	// ディレクトリのファイル数と一致するか簡易検証。不一致なら再構築が必要。
	files, err := os.ReadDir(h.dir)
	if err != nil {
		return nil, false
	}
	validCount := 0
	for _, f := range files {
		if id, ok := strings.CutSuffix(f.Name(), ".json"); ok && !f.IsDir() && idPattern.MatchString(id) && f.Name() != indexFileName {
			validCount++
		}
	}
	if len(headers) != validCount {
		return nil, false
	}
	entries := make([]Entry, 0, len(headers))
	for _, hdr := range headers {
		entries = append(entries, Entry{
			ID:         hdr.ID,
			DetectedAt: hdr.DetectedAt,
			OldSerial:  hdr.OldSerial,
			NewSerial:  hdr.NewSerial,
			Summary:    hdr.Summary,
		})
	}
	// 念のためソート（インデックスは降順で保存している想定）。
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	return entries, true
}

func (h *History) writeIndex(entries []Entry) error {
	headers := make([]entryHeader, 0, len(entries))
	for _, e := range entries {
		headers = append(headers, entryHeader{
			ID:         e.ID,
			DetectedAt: e.DetectedAt,
			OldSerial:  e.OldSerial,
			NewSerial:  e.NewSerial,
			Summary:    e.Summary,
		})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].ID > headers[j].ID })
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(h.dir, ".tmp-index-*")
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
	return os.Rename(tmpName, h.indexPath())
}

func (h *History) updateIndex(e Entry) error {
	// 既存インデックスを読み、追加して再保存。存在しなければ List 経由で再構築。
	headers, ok := h.readIndexIfValid()
	if !ok {
		// フォールバック: 全件読み直して再構築（List が header のみ読むので高速）
		entries, err := h.listWithoutIndex()
		if err != nil {
			return err
		}
		return h.writeIndex(entries)
	}
	// 追加: 既存に同 ID があれば置換、なければ追加
	found := false
	for i, hdr := range headers {
		if hdr.ID == e.ID {
			headers[i] = e
			// List では Changes は不要なのでクリアしておく
			headers[i].Changes = nil
			found = true
			break
		}
	}
	if !found {
		eh := e
		eh.Changes = nil
		headers = append(headers, eh)
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].ID > headers[j].ID })
	return h.writeIndex(headers)
}

// listWithoutIndex はインデックスを使わずにヘッダのみで一覧を作る内部ヘルパ。
func (h *History) listWithoutIndex() ([]Entry, error) {
	files, err := os.ReadDir(h.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(files))
	for _, f := range files {
		id, isJSON := strings.CutSuffix(f.Name(), ".json")
		if f.IsDir() || !isJSON || !idPattern.MatchString(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		e, err := h.readEntryHeader(id)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func cloneEntries(in []Entry) []Entry {
	if in == nil {
		return nil
	}
	out := make([]Entry, len(in))
	for i := range in {
		out[i] = in[i]
		// Summary.ByCategory は map なので深くコピー
		if in[i].Summary.ByCategory != nil {
			m := make(map[string]int, len(in[i].Summary.ByCategory))
			for k, v := range in[i].Summary.ByCategory {
				m[k] = v
			}
			out[i].Summary.ByCategory = m
		}
		// Changes は List では nil のはずだが、念のためスライスもコピー
		if in[i].Changes != nil {
			out[i].Changes = append([]ChangeJSON(nil), in[i].Changes...)
		}
	}
	return out
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

func (h *History) readEntryHeader(id string) (Entry, error) {
	data, err := os.ReadFile(filepath.Join(h.dir, id+".json"))
	if err != nil {
		return Entry{}, fmt.Errorf("read history entry %s: %w", id, err)
	}
	var hdr entryHeader
	if err := json.Unmarshal(data, &hdr); err != nil {
		return Entry{}, fmt.Errorf("parse history entry %s: %w", id, err)
	}
	// Changes は一覧では不要なので nil のまま返す。呼び出し側は Summary のみ使う。
	return Entry{
		ID:         hdr.ID,
		DetectedAt: hdr.DetectedAt,
		OldSerial:  hdr.OldSerial,
		NewSerial:  hdr.NewSerial,
		Summary:    hdr.Summary,
		Changes:    nil,
	}, nil
}
