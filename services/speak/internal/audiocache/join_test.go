package audiocache

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

func wav(pcm []byte, rate int) tts.Audio {
	return tts.Audio{Data: tts.WAV(pcm, rate, 1), ContentType: tts.ContentTypeWAV}
}

// withListChunk inserts a LIST chunk between a WAV's fmt and data chunks,
// the way encoders that write metadata lay a file out.
func withListChunk(a tts.Audio) tts.Audio {
	// tts.WAV writes a 44-byte header: RIFF header and fmt chunk end at 36.
	const fmtEnd = 36
	list := []byte("LIST\x05\x00\x00\x00INFOx\x00") // odd body, so a pad byte
	data := append(append(append([]byte(nil), a.Data[:fmtEnd]...), list...), a.Data[fmtEnd:]...)
	return tts.Audio{Data: data, ContentType: a.ContentType}
}

func TestJoinWAVs(t *testing.T) {
	first, second := []byte{1, 2, 3, 4}, []byte{5, 6, 7, 8, 9, 10}
	got, err := Join([]tts.Audio{wav(first, 24000), withListChunk(wav(second, 24000))})
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if got.ContentType != tts.ContentTypeWAV {
		t.Errorf("ContentType = %q, want %s", got.ContentType, tts.ContentTypeWAV)
	}
	if riff := binary.LittleEndian.Uint32(got.Data[4:8]); int(riff) != len(got.Data)-8 {
		t.Errorf("RIFF size = %d, want %d (file length - 8)", riff, len(got.Data)-8)
	}
	format, data, err := parseWAV(got.Data)
	if err != nil {
		t.Fatalf("joined clip does not parse: %v", err)
	}
	wantFormat, _, _ := parseWAV(tts.WAV(nil, 24000, 1))
	if !bytes.Equal(format, wantFormat) {
		t.Errorf("fmt chunk = %x, want the clips' own %x", format, wantFormat)
	}
	if want := append(append([]byte(nil), first...), second...); !bytes.Equal(data, want) {
		t.Errorf("samples = %v, want %v in order", data, want)
	}
}

func TestJoinRejects(t *testing.T) {
	mp3 := tts.Audio{Data: []byte("ID3a"), ContentType: tts.ContentTypeMP3}
	cases := []struct {
		name  string
		clips []tts.Audio
		want  string
	}{
		{"no clips", nil, "no clips"},
		{
			"mismatched fmt",
			[]tts.Audio{wav([]byte{1, 2}, 24000), wav([]byte{3, 4}, 16000)},
			"different WAV format",
		},
		{"WAV and MP3", []tts.Audio{wav([]byte{1, 2}, 24000), mp3}, "cannot be mixed"},
		{
			"not a WAV",
			[]tts.Audio{{Data: []byte("garbage"), ContentType: tts.ContentTypeWAV}},
			"not a WAV",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Join(tc.clips); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Join err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestJoinMP3sAppends(t *testing.T) {
	got, err := Join([]tts.Audio{
		{Data: []byte("ID3a"), ContentType: tts.ContentTypeMP3},
		{Data: []byte("ID3b"), ContentType: tts.ContentTypeMP3},
	})
	if err != nil || string(got.Data) != "ID3aID3b" || got.ContentType != tts.ContentTypeMP3 {
		t.Errorf("Join = %q %s, %v; want the streams appended as MP3", got.Data,
			got.ContentType, err)
	}
}
