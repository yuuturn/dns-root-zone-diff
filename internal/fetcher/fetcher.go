package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher は root zone ファイルを HTTP で取得する。
type Fetcher struct {
	url    string
	client *http.Client
}

// New は Fetcher を生成する。
func New(url string, timeout time.Duration) *Fetcher {
	return &Fetcher{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Fetch は zone ファイルを取得してバイト列を返す。
func (f *Fetcher) Fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch zone: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch zone: unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("fetch zone: empty response body")
	}

	return data, nil
}
