package audiocache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// WAV layout: a 12-byte RIFF header, then chunks of an 8-byte header (id,
// little-endian size) and a body padded to an even length.
const (
	riffHeaderLen   = 12
	chunkHeaderLen  = 8
	blockAlignField = 12 // offset of the block align field in a fmt chunk
)

// Join makes one clip of clips played in order: WAV clips become one WAV
// with a single data chunk, MP3 clips are appended frame stream to frame
// stream. WAV clips must share their exact format, and the formats cannot
// be mixed.
func Join(clips []tts.Audio) (tts.Audio, error) {
	if len(clips) == 0 {
		return tts.Audio{}, errors.New("audiocache: no clips to join")
	}
	ext := clips[0].Ext()
	for i, c := range clips {
		if c.Ext() != ext {
			return tts.Audio{}, fmt.Errorf(
				"audiocache: clip %d is %s but clip 1 is %s; formats cannot be mixed",
				i+1, c.Ext(), ext,
			)
		}
	}
	if ext == ".mp3" {
		var out bytes.Buffer
		for _, c := range clips {
			out.Write(c.Data)
		}
		return tts.Audio{Data: out.Bytes(), ContentType: tts.ContentTypeMP3}, nil
	}
	return joinWAV(clips)
}

// joinWAV concatenates the sample data of WAV clips under the first clip's
// fmt chunk. Other chunks (LIST metadata, say) are dropped.
func joinWAV(clips []tts.Audio) (tts.Audio, error) {
	var format []byte
	samples := make([][]byte, len(clips))
	total := 0
	for i, c := range clips {
		f, data, err := parseWAV(c.Data)
		if err != nil {
			return tts.Audio{}, fmt.Errorf("audiocache: clip %d: %w", i+1, err)
		}
		if format == nil {
			format = f
		} else if !bytes.Equal(f, format) {
			return tts.Audio{}, fmt.Errorf(
				"audiocache: clip %d has a different WAV format from clip 1", i+1,
			)
		}
		samples[i] = wholeFrames(data, format)
		total += len(samples[i])
	}
	fmtLen := chunkHeaderLen + padded(len(format))
	riffLen := 4 + fmtLen + chunkHeaderLen + padded(total) // 4 = "WAVE"
	if riffLen > math.MaxUint32 {
		return tts.Audio{}, errors.New("audiocache: joined audio exceeds the 4 GiB WAV limit")
	}
	var out bytes.Buffer
	out.Grow(chunkHeaderLen + riffLen)
	writeChunkHeader(&out, "RIFF", riffLen)
	out.WriteString("WAVE")
	writeChunk(&out, "fmt ", format)
	writeChunkHeader(&out, "data", total)
	for _, s := range samples {
		out.Write(s)
	}
	if total%2 == 1 {
		out.WriteByte(0)
	}
	return tts.Audio{Data: out.Bytes(), ContentType: tts.ContentTypeWAV}, nil
}

// parseWAV finds the fmt and data chunk bodies of a WAV file, stepping over
// any other chunks. A data chunk that claims more bytes than the file holds
// (a header written before the length was known) runs to the end of the file.
func parseWAV(b []byte) (format, data []byte, err error) {
	if len(b) < riffHeaderLen || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, nil, errors.New("not a WAV file")
	}
	for off := riffHeaderLen; off+chunkHeaderLen <= len(b) && data == nil; {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+chunkHeaderLen]))
		body := off + chunkHeaderLen
		end := body + size
		if end > len(b) {
			if id != "data" {
				return nil, nil, fmt.Errorf("WAV %q chunk runs past the end of the file", id)
			}
			end = len(b)
		}
		switch id {
		case "fmt ":
			format = b[body:end]
		case "data":
			data = b[body:end]
		}
		off = body + padded(size)
	}
	if format == nil || data == nil {
		return nil, nil, errors.New("WAV file has no fmt or no data chunk")
	}
	return format, data, nil
}

// wholeFrames trims data to whole sample frames, so a clip ending on a
// partial frame cannot shift every sample of the clips after it.
func wholeFrames(data, format []byte) []byte {
	if len(format) < blockAlignField+2 {
		return data
	}
	align := int(binary.LittleEndian.Uint16(format[blockAlignField:]))
	if align <= 1 {
		return data
	}
	return data[:len(data)-len(data)%align]
}

func writeChunk(out *bytes.Buffer, id string, body []byte) {
	writeChunkHeader(out, id, len(body))
	out.Write(body)
	if len(body)%2 == 1 {
		out.WriteByte(0)
	}
}

func writeChunkHeader(out *bytes.Buffer, id string, size int) {
	out.WriteString(id)
	_ = binary.Write(out, binary.LittleEndian, uint32(size))
}

// padded is a chunk body's length on disk: chunks start on even offsets.
func padded(n int) int {
	return n + n%2
}
