package gamesync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
)

// DefaultDotaconstantsBase serves the dotaconstants repository's raw files.
const DefaultDotaconstantsBase = "https://raw.githubusercontent.com"

// DefaultGitHubAPI resolves a ref to the commit it points at.
const DefaultGitHubAPI = "https://api.github.com"

// HTTPSource reads dotaconstants build files from GitHub at the commit a ref
// points to, resolved once. Not safe for concurrent use.
type HTTPSource struct {
	apiURL, baseURL, ref string
	client               *http.Client
	sha                  string
}

// NewHTTPSource builds a source; ref is a branch, tag or commit.
func NewHTTPSource(apiURL, baseURL, ref string, client *http.Client) *HTTPSource {
	return &HTTPSource{apiURL: strings.TrimRight(apiURL, "/"), baseURL: strings.TrimRight(baseURL, "/"), ref: ref, client: client}
}

var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Revision implements Source: the commit SHA the ref points to.
func (s *HTTPSource) Revision(ctx context.Context) (string, error) {
	if s.sha != "" {
		return s.sha, nil
	}
	url := fmt.Sprintf("%s/repos/odota/dotaconstants/commits/%s", s.apiURL, neturl.PathEscape(s.ref))
	b, err := s.get(ctx, url, "application/vnd.github.sha", 1<<10)
	if err != nil {
		return "", fmt.Errorf("resolve dotaconstants ref %q: %w", s.ref, err)
	}
	sha := strings.TrimSpace(string(b))
	if !commitSHA.MatchString(sha) {
		return "", fmt.Errorf("resolve dotaconstants ref %q: response is not a commit SHA", s.ref)
	}
	s.sha = sha
	return sha, nil
}

// Fetch implements Source.
func (s *HTTPSource) Fetch(ctx context.Context, file string) ([]byte, error) {
	sha, err := s.Revision(ctx)
	if err != nil {
		return nil, err
	}
	return s.get(ctx, fmt.Sprintf("%s/odota/dotaconstants/%s/build/%s", s.baseURL, sha, file), "", 32<<20)
}

func (s *HTTPSource) get(ctx context.Context, url, accept string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
