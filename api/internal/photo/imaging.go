package photo

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
)

// Jev's image limits (docs.typesafe.ai/images): each side 280 to 1568 px.
const (
	jevMinSide = 280
	jevMaxSide = 1568
)

// rgb is an image as float channels, row-major, for resizing and matching.
type rgb struct {
	w, h int
	px   []float32 // r, g, b per pixel
}

func toRGB(img image.Image) rgb {
	b := img.Bounds()
	out := rgb{w: b.Dx(), h: b.Dy(), px: make([]float32, 3*b.Dx()*b.Dy())}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			out.px[i], out.px[i+1], out.px[i+2] = float32(r>>8), float32(g>>8), float32(bb>>8)
			i += 3
		}
	}
	return out
}

// sub returns the part of m inside r (pixel coordinates), clipped to m.
func (m rgb) sub(r image.Rectangle) rgb {
	r = r.Intersect(image.Rect(0, 0, m.w, m.h))
	out := rgb{w: r.Dx(), h: r.Dy(), px: make([]float32, 3*r.Dx()*r.Dy())}
	for y := 0; y < out.h; y++ {
		copy(out.px[3*y*out.w:3*(y+1)*out.w], m.px[3*((r.Min.Y+y)*m.w+r.Min.X):3*((r.Min.Y+y)*m.w+r.Max.X)])
	}
	return out
}

// resize scales m to w x h: an area average when shrinking, bilinear when
// growing, so neither aliases.
func (m rgb) resize(w, h int) rgb {
	out := rgb{w: w, h: h, px: make([]float32, 3*w*h)}
	if m.w == 0 || m.h == 0 || w == 0 || h == 0 {
		return out
	}
	sx, sy := float64(m.w)/float64(w), float64(m.h)/float64(h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c [3]float32
			if sx > 1 || sy > 1 {
				c = m.average(float64(x)*sx, float64(y)*sy, float64(x+1)*sx, float64(y+1)*sy)
			} else {
				c = m.bilinear((float64(x)+0.5)*sx-0.5, (float64(y)+0.5)*sy-0.5)
			}
			copy(out.px[3*(y*w+x):], c[:])
		}
	}
	return out
}

func (m rgb) average(x0, y0, x1, y1 float64) [3]float32 {
	ix0, iy0 := int(x0), int(y0)
	ix1, iy1 := min(int(math.Ceil(x1)), m.w), min(int(math.Ceil(y1)), m.h)
	var sum [3]float32
	n := float32(0)
	for y := iy0; y < max(iy1, iy0+1); y++ {
		for x := ix0; x < max(ix1, ix0+1); x++ {
			p := m.px[3*(y*m.w+x):]
			sum[0], sum[1], sum[2] = sum[0]+p[0], sum[1]+p[1], sum[2]+p[2]
			n++
		}
	}
	return [3]float32{sum[0] / n, sum[1] / n, sum[2] / n}
}

func (m rgb) bilinear(x, y float64) [3]float32 {
	x = math.Max(0, math.Min(x, float64(m.w-1)))
	y = math.Max(0, math.Min(y, float64(m.h-1)))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, m.w-1), min(y0+1, m.h-1)
	fx, fy := float32(x-float64(x0)), float32(y-float64(y0))
	var c [3]float32
	for k := 0; k < 3; k++ {
		a := m.px[3*(y0*m.w+x0)+k]*(1-fx) + m.px[3*(y0*m.w+x1)+k]*fx
		b := m.px[3*(y1*m.w+x0)+k]*(1-fx) + m.px[3*(y1*m.w+x1)+k]*fx
		c[k] = a*(1-fy) + b*fy
	}
	return c
}

func (m rgb) image() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, m.w, m.h))
	for i := 0; i < m.w*m.h; i++ {
		img.Pix[4*i] = clamp8(m.px[3*i])
		img.Pix[4*i+1] = clamp8(m.px[3*i+1])
		img.Pix[4*i+2] = clamp8(m.px[3*i+2])
		img.Pix[4*i+3] = 255
	}
	return img
}

func clamp8(v float32) uint8 {
	return uint8(max(0, min(255, v+0.5)))
}

// forJev scales m so both sides fit Jev's limits, keeping the aspect ratio,
// and pads a side still under the minimum with black.
func (m rgb) forJev() *image.RGBA { return m.fit(jevMaxSide) }

// fit is forJev with a smaller longest side, maxSide, for an image whose
// details matter less: fewer bytes to upload.
func (m rgb) fit(maxSide int) *image.RGBA {
	s := 1.0
	if long := max(m.w, m.h); long > maxSide {
		s = float64(maxSide) / float64(long)
	}
	if short := min(m.w, m.h); float64(short)*s < jevMinSide {
		s = float64(jevMinSide) / float64(short)
	}
	w, h := max(1, int(math.Round(float64(m.w)*s))), max(1, int(math.Round(float64(m.h)*s)))
	w, h = min(w, jevMaxSide), min(h, jevMaxSide)
	scaled := m.resize(w, h).image()
	if w >= jevMinSide && h >= jevMinSide {
		return scaled
	}
	out := image.NewRGBA(image.Rect(0, 0, max(w, jevMinSide), max(h, jevMinSide)))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(out, scaled.Bounds(), scaled, image.Point{}, draw.Src)
	return out
}

// JPEG qualities: the photo sent to the vision model keeps its detail; the
// many small images of a Jev read trade some for upload size.
const (
	qualityPhoto = 88
	qualitySmall = 75
)

func encodeJPEG(img image.Image, quality int) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: quality}) // an in-memory RGBA always encodes
	return b.Bytes()
}
