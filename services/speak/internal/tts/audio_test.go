package tts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNormalize(t *testing.T) {
	pcm := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	cases := []struct {
		name        string
		data        []byte
		contentType string
		wantType    string
		wantRate    uint32
		wantChans   uint16
	}{
		{
			"wav passes through",
			[]byte("RIFF....WAVE"),
			"application/octet-stream",
			ContentTypeWAV,
			0,
			0,
		},
		{"mp3 by ID3 tag", []byte("ID3\x04"), "audio/mpeg", ContentTypeMP3, 0, 0},
		{"mp3 by frame sync", []byte{0xFF, 0xFB, 0x90}, "", ContentTypeMP3, 0, 0},
		{"pcm with rate", pcm, "audio/pcm; rate=22050; channels=2", ContentTypeWAV, 22050, 2},
		{"pcm defaults to 24k mono", pcm, "audio/pcm", ContentTypeWAV, 24000, 1},
		{"gemini L16", pcm, "audio/L16;codec=pcm;rate=24000", ContentTypeWAV, 24000, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.data, tc.contentType)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if got.ContentType != tc.wantType {
				t.Errorf("content type = %s, want %s", got.ContentType, tc.wantType)
			}
			if tc.wantRate == 0 {
				if !bytes.Equal(got.Data, tc.data) {
					t.Errorf("data changed on passthrough")
				}
				return
			}
			rate := binary.LittleEndian.Uint32(got.Data[24:28])
			chans := binary.LittleEndian.Uint16(got.Data[22:24])
			if rate != tc.wantRate || chans != tc.wantChans || !bytes.Equal(got.Data[44:], pcm) {
				t.Errorf("wav header rate=%d channels=%d, want %d/%d with the samples after it",
					rate, chans, tc.wantRate, tc.wantChans)
			}
		})
	}
}

func TestNormalizeRejectsWhatCannotPlay(t *testing.T) {
	for _, ct := range []string{"text/html", "application/json", ";;bad"} {
		if _, err := Normalize([]byte("<html>"), ct); err == nil {
			t.Errorf("Normalize(%q) accepted unplayable audio", ct)
		}
	}
}

func TestAudioExt(t *testing.T) {
	if (Audio{ContentType: ContentTypeMP3}).Ext() != ".mp3" ||
		(Audio{ContentType: ContentTypeWAV}).Ext() != ".wav" {
		t.Error("Ext must follow the content type afplay goes by")
	}
}
