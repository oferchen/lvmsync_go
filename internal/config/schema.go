package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

//go:embed schema.json
var configSchema []byte

func validateSchema(doc map[string]any) error {
	if len(configSchema) == 0 {
		return nil
	}
	schemaLoader := gojsonschema.NewBytesLoader(configSchema)
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("schema marshal: %w", err)
	}
	documentLoader := gojsonschema.NewBytesLoader(data)
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	if !result.Valid() {
		var parts []string
		for _, e := range result.Errors() {
			parts = append(parts, e.String())
		}
		return fmt.Errorf("schema validation: %s", strings.Join(parts, "; "))
	}
	return nil
}
