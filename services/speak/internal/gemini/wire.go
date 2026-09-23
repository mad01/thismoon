package gemini

import "github.com/mad01/thismoon/services/speak/internal/tts"

// generateRequest is the generateContent payload for speech: the text as
// the one user turn, audio as the only response modality, and a prebuilt
// voice. The API's JSON is camelCase.
type generateRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

// inlineData is audio in an answer: base64 bytes and their MIME type, such
// as audio/L16;codec=pcm;rate=24000.
type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type generationConfig struct {
	ResponseModalities []string     `json:"responseModalities"`
	SpeechConfig       speechConfig `json:"speechConfig"`
}

type speechConfig struct {
	VoiceConfig voiceConfig `json:"voiceConfig"`
}

type voiceConfig struct {
	PrebuiltVoiceConfig prebuiltVoiceConfig `json:"prebuiltVoiceConfig"`
}

type prebuiltVoiceConfig struct {
	VoiceName string `json:"voiceName"`
}

func newGenerateRequest(req tts.Request) generateRequest {
	return generateRequest{
		Contents: []content{{Parts: []part{{Text: req.Text}}}},
		GenerationConfig: generationConfig{
			ResponseModalities: []string{"AUDIO"},
			SpeechConfig: speechConfig{
				VoiceConfig: voiceConfig{
					PrebuiltVoiceConfig: prebuiltVoiceConfig{VoiceName: req.Voice},
				},
			},
		},
	}
}

// generateResponse is the part of a generateContent answer speak reads.
type generateResponse struct {
	Candidates []struct {
		Content      content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// errorBody is Google's error answer: {"error": {"code", "message",
// "status", "details": [{"reason"}]}}.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}
