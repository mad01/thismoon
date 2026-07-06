package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// ParseEntities decodes one or more YAML documents (separated by '---') into
// entities. Empty documents are skipped. Each entity's apiVersion defaults to
// DefaultAPIVersion when omitted. Entities are validated; the first invalid
// document yields an error identifying its position.
func ParseEntities(data []byte) ([]Entity, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var entities []Entity
	for i := 0; ; i++ {
		var e Entity
		err := dec.Decode(&e)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", i, err)
		}
		// Skip blank documents (e.g. a trailing '---' or comment-only doc).
		if e.Kind == "" && e.Metadata.Name == "" {
			continue
		}
		if e.APIVersion == "" {
			e.APIVersion = DefaultAPIVersion
		}
		if err := e.Validate(); err != nil {
			return nil, fmt.Errorf("document %d: %w", i, err)
		}
		entities = append(entities, e)
	}
	return entities, nil
}
