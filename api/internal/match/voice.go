package match

import (
	"context"
	"encoding/binary"
	"errors"
	"time"
)

// Transcriber turns a player's voice clip into text (ADR-0005).
type Transcriber interface {
	Transcribe(ctx context.Context, wav []byte, lang string) (string, error)
}

// MaxClip is the longest voice clip the coach takes.
const MaxClip = 30 * time.Second

// MaxClipBytes bounds an upload: MaxClip of 16 kHz mono 16-bit PCM, as the
// browser records it, plus room for the header.
const MaxClipBytes = 16000*2*30 + 4<<10

var (
	// ErrNotWAV means the clip is not 16-bit PCM WAV.
	ErrNotWAV = errors.New("the clip must be 16-bit PCM WAV")
	// ErrClipTooLong means the clip runs over MaxClip.
	ErrClipTooLong = errors.New("the clip is too long")
)

// CheckClip holds a clip to the WAV the browser sends: RIFF/WAVE with a
// PCM fmt chunk of 1 or 2 channels at 16 bits, 8 to 48 kHz, and a data
// chunk of at most MaxClip.
func CheckClip(b []byte) error {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return ErrNotWAV
	}
	var bytesPerSecond uint32
	for p := 12; p+8 <= len(b); {
		id, size := string(b[p:p+4]), binary.LittleEndian.Uint32(b[p+4:p+8])
		body := b[p+8:]
		switch id {
		case "fmt ":
			if bytesPerSecond != 0 { // decoders read the first fmt chunk
				break
			}
			if size < 16 || len(body) < 16 {
				return ErrNotWAV
			}
			format, channels := binary.LittleEndian.Uint16(body), binary.LittleEndian.Uint16(body[2:])
			rate, bits := binary.LittleEndian.Uint32(body[4:]), binary.LittleEndian.Uint16(body[14:])
			if format != 1 || (channels != 1 && channels != 2) || bits != 16 || rate < 8000 || rate > 48000 {
				return ErrNotWAV
			}
			bytesPerSecond = rate * uint32(channels) * 2
		case "data":
			if bytesPerSecond == 0 {
				return ErrNotWAV
			}
			n := min(uint64(size), uint64(len(body)))
			if n > uint64(MaxClip/time.Second)*uint64(bytesPerSecond) {
				return ErrClipTooLong
			}
			return nil
		}
		p += 8 + int(size) + int(size&1)
		if p < 0 {
			return ErrNotWAV
		}
	}
	return ErrNotWAV
}
