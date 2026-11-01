package codex

import (
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func TestPreparedRejectsOmittedMediaChannelsWithoutChangingRawMetadata(t *testing.T) {
	for _, kind := range []string{"response.audio.delta", "response.output_audio.delta", "response.output_audio_transcript.done", "response.video.delta", "response.image.delta"} {
		wire := frame(kind, map[string]any{"delta": "PRIVATE-MEDIA"}) + complete(msg("public"))
		events, _, e := parsedPrepared(wire)
		f, ok := provider.FailureOf(e)
		if !ok || f.Category() != provider.UnsupportedChannel {
			t.Fatal("private prepared path ignored unsupported media channel")
		}
		assertNoCalls(t, events)
		if _, e := parseStream(strings.NewReader(wire), func(provider.Event) error { return nil }); e != nil {
			t.Fatal("raw additive-metadata compatibility changed")
		}
	}
}
