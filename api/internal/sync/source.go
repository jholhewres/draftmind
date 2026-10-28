package sync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultDotaconstantsBase serves the dotaconstants repository's raw files.
const DefaultDotaconstantsBase = "https://raw.githubusercontent.com"

// HTTPSource reads dotaconstants build files from GitHub at a ref.
type HTTPSource struct {
	baseURL, ref string
	client       *http.Client
}

// NewHTTPSource builds a source; ref is a branch, tag or commit.
func NewHTTPSource(baseURL, ref string, client *http.Client) *HTTPSource {
	return &HTTPSource{baseURL: strings.TrimRight(baseURL, "/"), ref: ref, client: client}
}

// Fetch implements Source.
func (s *HTTPSource) Fetch(ctx context.Context, file string) ([]byte, error) {
	url := fmt.Sprintf("%s/odota/dotaconstants/%s/build/%s", s.baseURL, s.ref, file)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}
