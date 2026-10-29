package photo

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = c.R, c.G, c.B, 255
	}
	return img
}

func TestImagesSentToJevFitItsSizeLimits(t *testing.T) {
	cases := []struct{ w, h, wantW, wantH int }{
		{1600, 900, 1568, 882}, // a phone photo, downscaled by the browser
		{256, 144, 498, 280},   // a CDN portrait
		{60, 34, 494, 280},     // a slot crop from a far-away photo
		{1000, 700, 1000, 700}, // already inside the limits
	}
	for _, c := range cases {
		got := toRGB(solid(c.w, c.h, color.RGBA{200, 40, 40, 255})).forJev().Bounds()
		if got.Dx() != c.wantW || got.Dy() != c.wantH {
			t.Errorf("%dx%d became %dx%d, want %dx%d", c.w, c.h, got.Dx(), got.Dy(), c.wantW, c.wantH)
		}
	}
}

func TestResizeKeepsAFlatColour(t *testing.T) {
	m := toRGB(solid(97, 55, color.RGBA{10, 120, 230, 255}))
	for _, size := range [][2]int{{16, 9}, {300, 170}} {
		r := m.resize(size[0], size[1])
		for i := 0; i < r.w*r.h; i++ {
			if r.px[3*i] < 9.5 || r.px[3*i] > 10.5 || r.px[3*i+2] < 229.5 || r.px[3*i+2] > 230.5 {
				t.Fatalf("pixel %d of %v is %v", i, size, r.px[3*i:3*i+3])
			}
		}
	}
}
