package httpapi

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/gamesync"
)

func (s *Server) heroDetail(w http.ResponseWriter, r *http.Request) {
	lang := pickLang(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	d, err := s.catalog.HeroDetail(r.Context(), r.PathValue("slug"), lang)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such hero on the current patch"})
		return
	}
	if err != nil {
		s.log.Error("hero detail", "slug", r.PathValue("slug"), "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the hero"})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// countersShown is how many heroes each side of a hero's counters lists.
const countersShown = 5

func (s *Server) heroCounters(w http.ResponseWriter, r *http.Request) {
	c, err := s.catalog.HeroCounters(r.Context(), r.PathValue("slug"), countersShown)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such hero on the current patch"})
		return
	}
	if err != nil {
		s.log.Error("hero counters", "slug", r.PathValue("slug"), "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the hero's counters"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// pickLang chooses a synced language: the lang query parameter, else the
// Accept-Language header by weight, else catalog.FallbackLang. A tag
// matches exactly or by its primary language (pt-PT gets pt-BR).
func pickLang(query, acceptLanguage string) string {
	if l, ok := matchLang(query); ok {
		return l
	}
	type weighted struct {
		tag string
		q   float64
	}
	var tags []weighted
	for part := range strings.SplitSeq(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		if tag != "" && q > 0 {
			tags = append(tags, weighted{tag, q})
		}
	}
	slices.SortStableFunc(tags, func(a, b weighted) int { return cmp.Compare(b.q, a.q) })
	for _, t := range tags {
		if l, ok := matchLang(t.tag); ok {
			return l
		}
	}
	return catalog.FallbackLang
}

func matchLang(tag string) (string, bool) {
	if tag == "" {
		return "", false
	}
	for _, l := range gamesync.FeedLanguages {
		if strings.EqualFold(l.Tag, tag) {
			return l.Tag, true
		}
	}
	primary := func(t string) string { p, _, _ := strings.Cut(strings.ToLower(t), "-"); return p }
	for _, l := range gamesync.FeedLanguages {
		if primary(l.Tag) == primary(tag) {
			return l.Tag, true
		}
	}
	return "", false
}
