// Package web は diff 履歴を閲覧するための HTTP API と静的ファイル配信を提供する。
package web

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/yfujii/dns-root-diff/internal/store"
)

const (
	defaultPerPage = 20
	maxPerPage     = 100
	// maxPage は page * perPage の整数オーバーフローを防ぐための上限。
	// 実際のエントリ数 (年間数百件) に対して十分大きい。
	maxPage = 1_000_000
)

// HistoryReader は Server が必要とする履歴の読み取り操作。
type HistoryReader interface {
	List() ([]store.Entry, error)
	Get(id string) (store.Entry, error)
}

// Server は diff 閲覧用の HTTP ハンドラ群を提供する。
type Server struct {
	history       HistoryReader // root zone の差分履歴
	anchorHistory HistoryReader // root anchors の差分履歴
	static        fs.FS
}

// New は Server を生成する。static はフロントエンドのビルド成果物。
func New(history, anchorHistory HistoryReader, static fs.FS) *Server {
	return &Server{history: history, anchorHistory: anchorHistory, static: static}
}

// Handler はルーティング済みの http.Handler を返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/diffs", s.handleListDiffs)
	mux.HandleFunc("GET /api/diffs/{id}", s.handleGetDiff)
	mux.HandleFunc("GET /api/anchors/diffs", s.handleListAnchorDiffs)
	mux.HandleFunc("GET /api/anchors/diffs/{id}", s.handleGetAnchorDiff)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	// 未定義の API パスは SPA フォールバックさせず 404 を返す。
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.HandleFunc("GET /", s.handleStatic)
	return mux
}

// entrySummary は一覧 API 用に Changes を除いた Entry 表現。
type entrySummary struct {
	ID         string        `json:"id"`
	DetectedAt string        `json:"detected_at"`
	OldSerial  string        `json:"old_serial"`
	NewSerial  string        `json:"new_serial"`
	Summary    store.Summary `json:"summary"`
}

type listResponse struct {
	Diffs   []entrySummary `json:"diffs"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
}

func (s *Server) handleListDiffs(w http.ResponseWriter, r *http.Request) {
	s.handleList(w, r, s.history)
}

func (s *Server) handleListAnchorDiffs(w http.ResponseWriter, r *http.Request) {
	s.handleList(w, r, s.anchorHistory)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request, history HistoryReader) {
	entries, err := history.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list diffs")
		return
	}

	// ETag: 一覧の内容が変わったときだけ変わる弱い ETag。
	// total + 先頭/末尾 ID のハッシュで十分（2時間に1回しか増えない）。
	etag := listETag(entries)
	if etag != "" {
		w.Header().Set("ETag", etag)
		// 10秒だけ共有キャッシュ可能（nginx proxy_cache 用）
		w.Header().Set("Cache-Control", "public, max-age=10, must-revalidate")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	page := positiveIntParam(r, "page", 1)
	if page > maxPage {
		page = maxPage
	}
	perPage := positiveIntParam(r, "per_page", defaultPerPage)
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	start := (page - 1) * perPage
	end := start + perPage
	if start > len(entries) {
		start = len(entries)
	}
	if end > len(entries) {
		end = len(entries)
	}

	diffs := make([]entrySummary, 0, end-start)
	for _, e := range entries[start:end] {
		diffs = append(diffs, entrySummary{
			ID:         e.ID,
			DetectedAt: e.DetectedAt.Format("2006-01-02T15:04:05Z07:00"),
			OldSerial:  e.OldSerial,
			NewSerial:  e.NewSerial,
			Summary:    e.Summary,
		})
	}

	writeJSON(w, http.StatusOK, listResponse{
		Diffs:   diffs,
		Total:   len(entries),
		Page:    page,
		PerPage: perPage,
	})
}

func listETag(entries []store.Entry) string {
	if len(entries) == 0 {
		return `W/"0"`
	}
	h := sha256.New()
	// total と先頭/末尾 ID を混ぜる。ID は時刻順なのでこれで変化を検知できる。
	_, _ = fmt.Fprintf(h, "%d|%s|%s", len(entries), entries[0].ID, entries[len(entries)-1].ID)
	sum := h.Sum(nil)
	// 短く 16 hex 文字に切り詰め
	return fmt.Sprintf(`W/"%x"`, sum[:8])
}

func (s *Server) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	s.handleGet(w, r, s.history)
}

func (s *Server) handleGetAnchorDiff(w http.ResponseWriter, r *http.Request) {
	s.handleGet(w, r, s.anchorHistory)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, history HistoryReader) {
	entry, err := history.Get(r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidID):
			writeError(w, http.StatusBadRequest, "invalid diff id")
		case errors.Is(err, fs.ErrNotExist):
			writeError(w, http.StatusNotFound, "diff not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to read diff")
		}
		return
	}
	etag := fmt.Sprintf(`W/"%s"`, entry.ID)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleStatic は埋め込みファイルを配信し、存在しないパスには index.html を返す
// (SPA のクライアントサイドルーティング用フォールバック)。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}

	f, err := s.static.Open(name)
	if err != nil {
		name = "index.html"
	} else {
		_ = f.Close()
	}

	// Vite の成果物はコンテンツハッシュ付きファイル名のため長期キャッシュできる。
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, s.static, name)
}

func positiveIntParam(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
