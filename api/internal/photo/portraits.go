package photo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

// portrait is one hero's picture as the matcher and Jev use it.
type portrait struct {
	hero  catalog.Hero
	thumb []float32 // normalized thumbnail for the shortlist
	jpeg  []byte    // scaled into Jev's limits, for a choice's criteria
}

// Portraits fetches the heroes' portraits from the image CDN once, keeps
// them under Dir/portraits (outside git) and in memory.
type Portraits struct {
	dir, base string
	http      *http.Client
	mu        sync.Mutex
	loaded    map[string]*portrait // by image path
}

// NewPortraits caches portraits from base (the CDN the hero image paths are
// relative to) in dataDir/portraits.
func NewPortraits(dataDir, base string, client *http.Client) *Portraits {
	return &Portraits{dir: filepath.Join(dataDir, "portraits"), base: base, http: client, loaded: map[string]*portrait{}}
}

// Load fetches and caches every hero's portrait that is not cached yet.
func (p *Portraits) Load(ctx context.Context, heroes []catalog.Hero) error {
	_, err := p.byID(ctx, heroes)
	return err
}

// byID returns every hero's portrait by hero id, fetching the missing ones.
func (p *Portraits) byID(ctx context.Context, heroes []catalog.Hero) (map[int]*portrait, error) {
	out := make(map[int]*portrait, len(heroes))
	var missing []catalog.Hero
	p.mu.Lock()
	for _, h := range heroes {
		if pt, ok := p.loaded[h.Img]; ok {
			out[h.ID] = pt
		} else {
			missing = append(missing, h)
		}
	}
	p.mu.Unlock()
	if len(missing) == 0 {
		return out, nil
	}
	if err := os.MkdirAll(p.dir, 0o750); err != nil {
		return nil, fmt.Errorf("photo: portraits dir: %w", err)
	}
	type res struct {
		pt  *portrait
		err error
	}
	results := make([]res, len(missing))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, h := range missing {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pt, err := p.load(ctx, h)
			results[i] = res{pt, err}
		}()
	}
	wg.Wait()
	var errs []error
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		p.loaded[missing[i].Img] = r.pt
		out[missing[i].ID] = r.pt
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("photo: portraits: %w", err)
	}
	return out, nil
}

// maxPortrait bounds a portrait download; the CDN's are about 60 KB.
const maxPortrait = 2 << 20

func (p *Portraits) load(ctx context.Context, h catalog.Hero) (*portrait, error) {
	name := path.Base(h.Img)
	if name == "." || name == "/" || name == "" {
		return nil, fmt.Errorf("%s: no image path", h.Name)
	}
	root, err := os.OpenRoot(p.dir) // the file name cannot leave the cache dir
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(name)
	if err != nil {
		if data, err = p.fetch(ctx, h.Img); err != nil {
			return nil, fmt.Errorf("%s: %w", h.Name, err)
		}
		if err := root.WriteFile(name, data, 0o600); err != nil {
			return nil, fmt.Errorf("%s: %w", h.Name, err)
		}
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.Name, err)
	}
	m := toRGB(img)
	return &portrait{hero: h, thumb: thumbnail(m), jpeg: encodeJPEG(m.forJev(), qualitySmall)}, nil
}

func (p *Portraits) fetch(ctx context.Context, imgPath string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+imgPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", imgPath, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxPortrait))
}
