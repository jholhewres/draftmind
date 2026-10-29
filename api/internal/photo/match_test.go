package photo

import (
	"image"
	"image/color"
	"image/draw"
	"math/rand/v2"
	"testing"
)

// noise is a random 256x144 picture standing in for a hero portrait.
func noise(seed uint64) *image.RGBA {
	r := rand.New(rand.NewPCG(seed, 1))
	img := image.NewRGBA(image.Rect(0, 0, 256, 144))
	// Broad blocks rather than pixels, like a portrait's areas of colour, so
	// the picture survives scaling.
	for by := 0; by < 144; by += 24 {
		for bx := 0; bx < 256; bx += 32 {
			c := color.RGBA{uint8(r.IntN(256)), uint8(r.IntN(256)), uint8(r.IntN(256)), 255}
			draw.Draw(img, image.Rect(bx, by, bx+32, by+24), image.NewUniform(c), image.Point{}, draw.Src)
		}
	}
	return img
}

func TestSearchShortlistsTheHeroInASlotAndFindsWhereItIs(t *testing.T) {
	ports := map[int]*portrait{}
	for id := 1; id <= 6; id++ {
		ports[id] = &portrait{thumb: thumbnail(toRGB(noise(uint64(id))))}
	}
	// A dim photo with hero 4's portrait at 300,40, 120x68 px, under a glare.
	photo := image.NewRGBA(image.Rect(0, 0, 800, 300))
	draw.Draw(photo, photo.Bounds(), image.NewUniform(color.RGBA{20, 20, 25, 255}), image.Point{}, draw.Src)
	placed := image.Rect(300, 40, 420, 108)
	draw.Draw(photo, placed, toRGB(noise(4)).resize(120, 68).image(), image.Point{}, draw.Src)
	m := toRGB(photo)
	for i := range m.px {
		m.px[i] = m.px[i]*0.8 + 30
	}
	// The guess is 25 px off and 10 % too small.
	f := search(m, image.Rect(325, 30, 433, 91), ports, []float64{0.9, 1, 1.1, 1.2}, 0.45)
	if f.Ranks[0].ID != 4 || f.Ranks[0].Score < 0.8 {
		t.Fatalf("ranks %+v", f.Ranks)
	}
	if f.Ranks[1].Score > 0.5 {
		t.Fatalf("another hero fits as well: %+v", f.Ranks)
	}
	d := func(a, b int) int { return max(a-b, b-a) }
	if d(f.Rect.Min.X, placed.Min.X) > 8 || d(f.Rect.Min.Y, placed.Min.Y) > 6 || d(f.Rect.Dx(), placed.Dx()) > 16 {
		t.Fatalf("found %v, placed at %v", f.Rect, placed)
	}
}
