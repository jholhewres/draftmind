package writer

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// ErrNoSpeech means the clip held no words to transcribe.
var ErrNoSpeech = errors.New("the clip has no speech")

const sttInstructions = "You transcribe one short voice message a player records for their coach during a 5v5 hero " +
	"match. The player most likely speaks the language whose BCP 47 tag is %s, but write what they say in the language " +
	"they speak. Answer with the transcript only, verbatim, as plain text: no quotes, no notes, no translation. Spell " +
	"hero, ability and item names as the game does. When the clip has no speech, answer with nothing."

// Transcribe turns a WAV clip into text with an audio-input chat model
// (ADR-0005). An empty transcript returns ErrNoSpeech; a late answer
// returns an error wrapping context.DeadlineExceeded.
func (w *Writer) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, CoachTimeout)
	defer cancel()
	start := time.Now()
	call := Call{Purpose: "coach.stt", Model: w.Model}
	parts := []any{
		map[string]any{"type": "text", "text": "Transcribe this clip."},
		map[string]any{"type": "input_audio", "input_audio": map[string]string{
			"data": base64.StdEncoding.EncodeToString(wav), "format": "wav",
		}},
	}
	text, err := w.chat(ctx, fmt.Sprintf(sttInstructions, lang), parts, 300, false, &call)
	call.Latency = time.Since(start)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		call.Outcome, call.Error = OutcomeTimeout, err.Error()
	case err != nil:
		call.Outcome, call.Error = OutcomeError, err.Error()
	case text == "":
		call.Outcome, err = OutcomeRejected, ErrNoSpeech
		call.Error = err.Error()
	default:
		call.Outcome, call.Output = OutcomeOK, text
	}
	w.record(ctx, call)
	return text, err
}
