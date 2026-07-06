package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/options"
	"github.com/knights-analytics/hugot/pipelines"
)

const (
	// hugotModelName is the sentence-transformer model used for embeddings.
	hugotModelName = "sentence-transformers/all-MiniLM-L6-v2"
	// hugotModelSubdir is the directory hugot.DownloadModel creates under the
	// model cache dir (the model name with "/" replaced by "_"). The model
	// files (tokenizer.json, onnx/) live here, not in the parent cache dir.
	hugotModelSubdir = "sentence-transformers_all-MiniLM-L6-v2"
	// hugotOnnxFilename is the quantized ONNX weight file within the model dir.
	hugotOnnxFilename = "onnx/model_qint8_arm64.onnx"
	// hugotDim is the embedding dimensionality of the model above.
	hugotDim = 384
)

// DefaultORTLibDir returns the directory holding the onnxruntime shared library
// (libonnxruntime.dylib) and the tokenizers static library (libtokenizers.a).
// The recipe prefetches both here; the Makefile links libtokenizers.a from it
// and HugotEmbedder loads the dylib from it at runtime.
func DefaultORTLibDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "csl", "ortlib"), nil
}

// HugotEmbedder embeds text with a local ONNX sentence-transformer via hugot's
// onnxruntime (ORT) backend. The model must already be present on disk under
// modelDir and the onnxruntime shared library under DefaultORTLibDir;
// HugotEmbedder never downloads either. Build with -tags ORT.
type HugotEmbedder struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
}

// NewHugotEmbedder opens an onnxruntime session and a feature-extraction
// pipeline over the model already downloaded to modelDir. Call Close when done.
func NewHugotEmbedder(ctx context.Context, modelDir string) (*HugotEmbedder, error) {
	libDir, err := DefaultORTLibDir()
	if err != nil {
		return nil, err
	}
	session, err := hugot.NewORTSession(ctx, options.WithOnnxLibraryPath(libDir))
	if err != nil {
		return nil, fmt.Errorf("create onnxruntime session (is the binary built with -tags ORT and the dylib in %s?): %w", libDir, err)
	}

	modelPath := filepath.Join(modelDir, hugotModelSubdir)
	cfg := hugot.FeatureExtractionConfig{
		ModelPath:    modelPath,
		OnnxFilename: hugotOnnxFilename,
		Name:         "embed",
	}
	pipeline, err := hugot.NewPipeline(session, cfg)
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("create feature-extraction pipeline for %s: %w", modelPath, err)
	}

	return &HugotEmbedder{session: session, pipeline: pipeline}, nil
}

// Embed returns one 384-dim vector per input text, preserving order. The ORT
// backend truncates inputs past the model's 512-token limit automatically, so
// over-length chunks degrade gracefully rather than failing.
func (h *HugotEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out, err := h.pipeline.RunPipeline(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("run feature-extraction pipeline: %w", err)
	}
	return out.Embeddings, nil
}

// Dim returns the model's embedding dimensionality (384).
func (h *HugotEmbedder) Dim() int { return hugotDim }

// Close destroys the underlying hugot session.
func (h *HugotEmbedder) Close() error {
	if h.session == nil {
		return nil
	}
	if err := h.session.Destroy(); err != nil {
		return fmt.Errorf("destroy hugot session: %w", err)
	}
	return nil
}
