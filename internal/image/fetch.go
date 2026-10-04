package image

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher retrieves images over HTTP.
type Fetcher struct {
	client *http.Client
}

// FetchOption configures a Fetcher.
type FetchOption func(*Fetcher)

// WithTimeout sets the HTTP client timeout. The zero duration means no
// client-level timeout (the context still bounds the request).
func WithTimeout(d time.Duration) FetchOption {
	return func(f *Fetcher) {
		f.client.Timeout = d
	}
}

// NewFetcher returns a Fetcher backed by a default http.Client. The client
// follows redirects per http.DefaultTransport semantics; no custom
// redirect policy is applied.
func NewFetcher(opts ...FetchOption) *Fetcher {
	f := &Fetcher{client: &http.Client{}}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Fetch retrieves the image at url and returns its bytes. The request is
// bounded by ctx (cancellation and deadline) and, if set, the client
// timeout. Any non-2xx status is an error.
func (f *Fetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("image fetch: bad request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("image fetch %s: unexpected status %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("image fetch %s: reading body: %w", url, err)
	}
	return data, nil
}
