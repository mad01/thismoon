package tts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"mime"
	"strconv"
	"strings"
)

// PCM defaults for a raw audio answer that names no rate or channel count:
// 24 kHz mono, what OpenRouter's pcm format and the Gemini API send.
const (
	defaultPCMRate     = 24000
	defaultPCMChannels = 1
	pcmBitsPerSample   = 16
)

// Content types of playable audio. Everything a provider returns is
// normalized to one of these before it reaches the browser or afplay.
const (
	ContentTypeWAV = "audio/wav"
	ContentTypeMP3 = "audio/mpeg"
)

// Request is one synthesis call: a chunk of text (a sentence, in practice)
// in a voice. Speed is a multiplier where 1 is normal; 0 leaves it to the
// provider.
type Request struct {
	Text  string
	Voice string
	Speed float64
}

// Audio is a synthesized clip ready to play: WAV or MP3 bytes and the
// matching content type.
type Audio struct {
	Data        []byte
	ContentType string
}

// Ext is the file extension afplay needs to recognize the clip.
func (a Audio) Ext() string {
	if a.ContentType == ContentTypeMP3 {
		return ".mp3"
	}
	return ".wav"
}

// Normalize turns a provider's answer into playable Audio. WAV and MP3 pass
// through; raw 16-bit PCM (audio/pcm, audio/L16) gets a WAV header built
// from the rate and channels parameters of contentType, defaulting to
// 24 kHz mono. The bytes decide before the header does, since a backend
// behind a proxy can label WAV as octet-stream.
func Normalize(data []byte, contentType string) (Audio, error) {
	switch {
	case bytes.HasPrefix(data, []byte("RIFF")):
		return Audio{Data: data, ContentType: ContentTypeWAV}, nil
	case isMP3(data):
		return Audio{Data: data, ContentType: ContentTypeMP3}, nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return Audio{}, fmt.Errorf("tts: unreadable audio content type %q: %w", contentType, err)
	}
	switch strings.ToLower(mediaType) {
	case "audio/pcm", "audio/l16", "audio/x-pcm":
		rate := paramInt(params, "rate", defaultPCMRate)
		channels := paramInt(params, "channels", defaultPCMChannels)
		return Audio{Data: WAV(data, rate, channels), ContentType: ContentTypeWAV}, nil
	default:
		return Audio{}, fmt.Errorf("tts: unsupported audio format %q", contentType)
	}
}

// WAV wraps raw little-endian 16-bit PCM samples in a 44-byte WAV header.
func WAV(pcm []byte, rate, channels int) []byte {
	blockAlign := channels * pcmBitsPerSample / 8
	var buf bytes.Buffer
	buf.Grow(44 + len(pcm))
	buf.WriteString("RIFF")
	le32(&buf, uint32(36+len(pcm)))
	buf.WriteString("WAVEfmt ")
	le32(&buf, 16) // fmt chunk size
	le16(&buf, 1)  // PCM
	le16(&buf, uint16(channels))
	le32(&buf, uint32(rate))
	le32(&buf, uint32(rate*blockAlign)) // byte rate
	le16(&buf, uint16(blockAlign))
	le16(&buf, pcmBitsPerSample)
	buf.WriteString("data")
	le32(&buf, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// isMP3 reports an ID3 tag or an MPEG frame sync at the start of data.
func isMP3(data []byte) bool {
	if bytes.HasPrefix(data, []byte("ID3")) {
		return true
	}
	return len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0
}

func paramInt(params map[string]string, key string, fallback int) int {
	if n, err := strconv.Atoi(params[key]); err == nil && n > 0 {
		return n
	}
	return fallback
}

func le16(buf *bytes.Buffer, v uint16) { _ = binary.Write(buf, binary.LittleEndian, v) }
func le32(buf *bytes.Buffer, v uint32) { _ = binary.Write(buf, binary.LittleEndian, v) }
