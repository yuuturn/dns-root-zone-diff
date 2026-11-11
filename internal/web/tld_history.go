package web

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/yfujii/dns-root-diff/internal/tldhistory"
)

// TLDHistoryReader は Server が必要とする TLD 別履歴の読み取り操作。
type TLDHistoryReader interface {
	List() ([]tldhistory.Summary, error)
	Get(tld string) (tldhistory.Detail, error)
}

// tldHistorySummaryJSON は TLD 履歴一覧 API の 1 要素。
type tldHistorySummaryJSON struct {
	TLD             string         `json:"tld"`
	FirstDetectedAt string         `json:"first_detected_at"`
	LastDetectedAt  string         `json:"last_detected_at"`
	Events          int            `json:"events"`
	Changes         int            `json:"changes"`
	ByCategory      map[string]int `json:"by_category"`
}

type tldHistoryListResponse struct {
	TLDs    []tldHistorySummaryJSON `json:"tlds"`
	Total   int                     `json:"total"`
	Page    int                     `json:"page"`
	PerPage int                     `json:"per_page"`
}

// tldHistoryStatsJSON は TLD 詳細の集計 (全イベント分)。
type tldHistoryStatsJSON struct {
	Events          int            `json:"events"`
	Changes         int            `json:"changes"`
	FirstDetectedAt string         `json:"first_detected_at"`
	LastDetectedAt  string         `json:"last_detected_at"`
	ByCategory      map[string]int `json:"by_category"`
}

type tldHistoryDetailResponse struct {
	TLD     string              `json:"tld"`
	Stats   tldHistoryStatsJSON `json:"stats"`
	Events  []tldhistory.Event  `json:"events"`
	Total   int                 `json:"total"`
	Page    int                 `json:"page"`
	PerPage int                 `json:"per_page"`
}

func (s *Server) handleListTLDHistory(w http.ResponseWriter, r *http.Request) {
	var summaries []tldhistory.Summary
	if s.tldZoneHistory != nil {
		var err error
		summaries, err = s.tldZoneHistory.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list TLD history")
			return
		}
	}

	etag := tldHistoryListETag(summaries)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=10, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
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
	if start > len(summaries) {
		start = len(summaries)
	}
	if end > len(summaries) {
		end = len(summaries)
	}

	tlds := make([]tldHistorySummaryJSON, 0, end-start)
	for _, s := range summaries[start:end] {
		tlds = append(tlds, tldHistorySummaryJSON{
			TLD:             s.TLD,
			FirstDetectedAt: formatHistoryTime(s.FirstDetectedAt),
			LastDetectedAt:  formatHistoryTime(s.LastDetectedAt),
			Events:          s.Events,
			Changes:         s.Changes,
			ByCategory:      nonNilCategories(s.ByCategory),
		})
	}

	writeJSON(w, http.StatusOK, tldHistoryListResponse{
		TLDs:    tlds,
		Total:   len(summaries),
		Page:    page,
		PerPage: perPage,
	})
}

func (s *Server) handleGetTLDHistory(w http.ResponseWriter, r *http.Request) {
	tld, err := tldhistory.NormalizeTLD(r.PathValue("tld"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tld")
		return
	}

	var detail tldhistory.Detail
	if s.tldZoneHistory != nil {
		detail, err = s.tldZoneHistory.Get(tld)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read TLD history")
			return
		}
	} else {
		detail = tldhistory.Detail{TLD: tld}
	}

	page := positiveIntParam(r, "page", 1)
	if page > maxPage {
		page = maxPage
	}
	perPage := positiveIntParam(r, "per_page", defaultDetailPerPage)
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	etag := tldHistoryDetailETag(tld, detail.Events, page, perPage)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	start := (page - 1) * perPage
	end := start + perPage
	if start > len(detail.Events) {
		start = len(detail.Events)
	}
	if end > len(detail.Events) {
		end = len(detail.Events)
	}

	events := make([]tldhistory.Event, 0, end-start)
	events = append(events, detail.Events[start:end]...)

	writeJSON(w, http.StatusOK, tldHistoryDetailResponse{
		TLD:     tld,
		Stats:   buildTLDHistoryStats(detail.Events),
		Events:  events,
		Total:   len(detail.Events),
		Page:    page,
		PerPage: perPage,
	})
}

func buildTLDHistoryStats(events []tldhistory.Event) tldHistoryStatsJSON {
	stats := tldHistoryStatsJSON{
		Events:     len(events),
		ByCategory: map[string]int{},
	}
	var first, last time.Time
	for _, ev := range events {
		stats.Changes += ev.Summary.Total
		for cat, n := range ev.Summary.ByCategory {
			stats.ByCategory[cat] += n
		}
		if first.IsZero() || ev.DetectedAt.Before(first) {
			first = ev.DetectedAt
		}
		if ev.DetectedAt.After(last) {
			last = ev.DetectedAt
		}
	}
	stats.FirstDetectedAt = formatHistoryTime(first)
	stats.LastDetectedAt = formatHistoryTime(last)
	return stats
}

func formatHistoryTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02T15:04:05Z07:00")
}

func nonNilCategories(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

func tldHistoryListETag(summaries []tldhistory.Summary) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d", len(summaries))
	for _, s := range summaries {
		_, _ = fmt.Fprintf(h, "|%s:%d:%d:%d", s.TLD, s.Events, s.Changes, s.LastDetectedAt.Unix())
	}
	sum := h.Sum(nil)
	return fmt.Sprintf(`W/"%x"`, sum[:8])
}

func tldHistoryDetailETag(tld string, events []tldhistory.Event, page, perPage int) string {
	firstID := ""
	if len(events) > 0 {
		firstID = events[0].ID
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d|%s|%d|%d", tld, len(events), firstID, page, perPage)
	sum := h.Sum(nil)
	return fmt.Sprintf(`W/"%x"`, sum[:8])
}
