package gamesync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

// githubStub answers the commits API for ref and serves files at testSHA.
func githubStub(t *testing.T, ref string, files map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/odota/dotaconstants/commits/"+ref:
			if r.Header.Get("Accept") != "application/vnd.github.sha" {
				http.Error(w, "want the sha media type", http.StatusUnsupportedMediaType)
				return
			}
			_, _ = w.Write([]byte(testSHA))
		case strings.HasPrefix(r.URL.Path, "/odota/dotaconstants/"+testSHA+"/build/"):
			body, ok := files[strings.TrimPrefix(r.URL.Path, "/odota/dotaconstants/"+testSHA+"/build/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPSourceResolvesTheRefToACommitSHA(t *testing.T) {
	srv := githubStub(t, "master", nil)
	rev, err := NewHTTPSource(srv.URL, srv.URL, "master", srv.Client()).Revision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rev != testSHA {
		t.Fatalf("revision %q, want %s", rev, testSHA)
	}
}

func TestHTTPSourceFetchesBuildFilesAtTheResolvedSHA(t *testing.T) {
	srv := githubStub(t, "master", map[string]string{"patch.json": `[]`})
	b, err := NewHTTPSource(srv.URL, srv.URL, "master", srv.Client()).Fetch(context.Background(), "patch.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("body %q, want []", b)
	}
}

func TestHTTPSourceRejectsAResponseThatIsNotASHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer srv.Close()

	_, err := NewHTTPSource(srv.URL, srv.URL, "no-such-branch", srv.Client()).Revision(context.Background())
	if err == nil || !strings.Contains(err.Error(), "commit SHA") {
		t.Fatalf("want a commit SHA error, got %v", err)
	}
}

func TestHTTPSourceFailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := NewHTTPSource(srv.URL, srv.URL, "master", srv.Client()).Fetch(context.Background(), "heroes.json")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want a 429 error, got %v", err)
	}
}
