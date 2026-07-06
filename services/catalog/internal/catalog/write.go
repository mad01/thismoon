package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RenderEntity serializes a single entity to YAML, stamping the default
// apiVersion when missing. The entity is validated first.
func RenderEntity(e Entity) ([]byte, error) {
	if e.APIVersion == "" {
		e.APIVersion = DefaultAPIVersion
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(e); err != nil {
		return nil, fmt.Errorf("encode entity: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// WriteServiceInfo writes one entity as service-info.yaml into dir, creating
// dir if needed. It refuses to overwrite an existing file so that an
// accidental add cannot clobber hand-written metadata.
func WriteServiceInfo(dir string, e Entity) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, ServiceInfoFile)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists; edit it directly instead of overwriting", path)
	}
	data, err := RenderEntity(e)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
