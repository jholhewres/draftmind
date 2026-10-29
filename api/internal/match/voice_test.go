package match

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// wav builds a WAV header with the given fmt fields and a data chunk that
// claims dataLen bytes but carries only body.
func wav(format, channels uint16, rate uint32, bits uint16, dataLen uint32, body []byte) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(body)))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(format)
	w(channels)
	w(rate)
	w(rate * uint32(channels) * uint32(bits) / 8)
	w(channels * bits / 8)
	w(bits)
	b.WriteString("data")
	w(dataLen)
	b.Write(body)
	return b.Bytes()
}

func TestCheckClipTakesOnlyShortPCMWAV(t *testing.T) {
	second := make([]byte, 32000)
	for _, c := range []struct {
		name string
		clip []byte
		want error
	}{
		{"a second of 16 kHz mono", wav(1, 1, 16000, 16, 32000, second), nil},
		{"not RIFF", []byte("OggS....WAVE"), ErrNotWAV},
		{"compressed, not PCM", wav(3, 1, 16000, 16, 32000, second), ErrNotWAV},
		{"8-bit samples", wav(1, 1, 16000, 8, 32000, second), ErrNotWAV},
		{"no data chunk", wav(1, 1, 16000, 16, 0, nil)[:36], ErrNotWAV},
		// A tiny body at a low rate: 31 s of 8 kHz mono is 496,000 bytes.
		{"over 30 s", wav(1, 1, 8000, 16, 496000, make([]byte, 496000)), ErrClipTooLong},
	} {
		if err := CheckClip(c.clip); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}
