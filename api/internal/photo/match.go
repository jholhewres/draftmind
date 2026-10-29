package photo

import (
	"image"
	"math"
	"slices"
)

// The shortlist compares small thumbnails: zero-mean per channel and unit
// length, so a dot product is their correlation, blind to exposure and to
// a colour cast.
const thumbW, thumbH = 16, 9

// thumbnail is m's normalized thumbnail, of its centre without a thin
// border that framing may cut.
func thumbnail(m rgb) []float32 {
	inX, inY := m.w/20, m.h/20
	return normalize(m.sub(image.Rect(inX, inY, m.w-inX, m.h-inY)).resize(thumbW, thumbH).px)
}

func normalize(v []float32) []float32 {
	out := make([]float32, len(v))
	var mean [3]float32
	n := float32(len(v) / 3)
	for i, x := range v {
		mean[i%3] += x / n
	}
	var norm float64
	for i, x := range v {
		out[i] = x - mean[i%3]
		norm += float64(out[i] * out[i])
	}
	if norm == 0 {
		return out // a flat patch correlates with nothing
	}
	inv := float32(1 / math.Sqrt(norm))
	for i := range out {
		out[i] *= inv
	}
	return out
}

func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// colourSpread is how far m's colours spread around their mean, apart
// from brightness: the deviation of its red-green and yellow-blue
// channels. A colour cast shifts their means and leaves it alone.
func colourSpread(m rgb) float32 {
	n := m.w * m.h
	if n == 0 {
		return 0
	}
	var s1, s2, q1, q2 float64
	for i := 0; i < n; i++ {
		r, g, b := float64(m.px[3*i]), float64(m.px[3*i+1]), float64(m.px[3*i+2])
		rg, yb := r-g, (r+g)/2-b
		s1, s2, q1, q2 = s1+rg, s2+yb, q1+rg*rg, q2+yb*yb
	}
	fn := float64(n)
	v := q1/fn - (s1/fn)*(s1/fn) + q2/fn - (s2/fn)*(s2/fn)
	return float32(math.Sqrt(max(v, 0)))
}

// inset is r without a 1/div margin on each side.
func inset(r image.Rectangle, div int) image.Rectangle {
	dx, dy := r.Dx()/div, r.Dy()/div
	return image.Rect(r.Min.X+dx, r.Min.Y+dy, r.Max.X-dx, r.Max.Y-dy)
}

// candidate is a hero and how well its portrait fits a slot, -1 to 1.
type candidate struct {
	ID    int
	Score float32
}

// found is where a slot's portrait best fits around a guessed position and
// every hero's best fit there, best first.
type found struct {
	Rect  image.Rectangle
	Best  float32
	Ranks []candidate
}

// search slides each portrait over the region around guess, at a few scales
// of its size, and keeps each hero's best correlation and the place of the
// best one overall. slack is how far, in slot sizes, the slot may sit from
// the guess.
func search(photo rgb, guess image.Rectangle, ports map[int]*portrait, scales []float64, slack float64) found {
	best := map[int]float32{}
	f := found{Rect: guess, Best: -2}
	patch := make([]float32, 3*thumbW*thumbH)
	for _, s := range scales {
		// The thumbnail is a portrait's centre, 90 % of each side.
		tw, th := 0.9*float64(guess.Dx())*s, 0.9*float64(guess.Dy())*s
		if tw < 4 || th < 4 {
			continue
		}
		cx, cy := float64(guess.Min.X+guess.Max.X)/2, float64(guess.Min.Y+guess.Max.Y)/2
		win := image.Rect(int(cx-tw/2-slack*tw), int(cy-th/2-slack*th), int(cx+tw/2+slack*tw), int(cy+th/2+slack*th)).
			Intersect(image.Rect(0, 0, photo.w, photo.h))
		k := thumbW / tw // photo pixels to thumbnail pixels
		ww, wh := int(math.Round(float64(win.Dx())*k)), int(math.Round(float64(win.Dy())*k))
		if ww < thumbW || wh < thumbH {
			continue
		}
		small := photo.sub(win).resize(ww, wh)
		for oy := 0; oy+thumbH <= wh; oy++ {
			for ox := 0; ox+thumbW <= ww; ox++ {
				for y := 0; y < thumbH; y++ {
					copy(patch[3*y*thumbW:3*(y+1)*thumbW], small.px[3*((oy+y)*ww+ox):3*((oy+y)*ww+ox+thumbW)])
				}
				p := normalize(patch)
				for id, pt := range ports {
					sc := dot(p, pt.thumb)
					if b, ok := best[id]; !ok || sc > b {
						best[id] = sc
					}
					if sc > f.Best {
						x0, y0 := float64(win.Min.X)+float64(ox)/k, float64(win.Min.Y)+float64(oy)/k
						// Grow the centre back to the whole portrait.
						inX, inY := tw/18, th/18
						f.Best = sc
						f.Rect = image.Rect(int(x0-inX), int(y0-inY), int(x0+tw+inX), int(y0+th+inY))
					}
				}
			}
		}
	}
	for id, sc := range best {
		f.Ranks = append(f.Ranks, candidate{ID: id, Score: sc})
	}
	slices.SortFunc(f.Ranks, func(a, b candidate) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return a.ID - b.ID
	})
	return f
}
