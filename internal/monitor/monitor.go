package monitor

import (
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// ContractMonitor validates incoming requests against a JSON schema.
type ContractMonitor struct {
	schemaLoader gojsonschema.JSONLoader
}

// NewContractMonitor creates a new ContractMonitor with the given schema file path.
// The schemaPath should be an absolute path or relative to the execution directory.
func NewContractMonitor(schemaPath string) (*ContractMonitor, error) {
	schemaLoader := gojsonschema.NewReferenceLoader("file://" + schemaPath)
	// Check if schema is valid by trying to load it into a schema object
	_, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return nil, fmt.Errorf("error loading or compiling schema %s: %w", schemaPath, err)
	}

	return &ContractMonitor{
		schemaLoader: schemaLoader,
	}, nil
}

// Validate validates the given request body against the loaded JSON schema.
// It returns true if valid, or false and a list of validation errors if invalid.
func (cm *ContractMonitor) Validate(requestBody []byte) (bool, []string, error) {
	documentLoader := gojsonschema.NewBytesLoader(requestBody)
	result, err := gojsonschema.Validate(cm.schemaLoader, documentLoader)
	if err != nil {
		return false, nil, fmt.Errorf("error during validation: %w", err)
	}

	if result.Valid() {
		return true, nil, nil
	}

	var errors []string
	for _, desc := range result.Errors() {
		errors = append(errors, desc.String())
	}
	return false, errors, nil
}

// FormatErrors formats a slice of validation error strings into a single string.
func FormatErrors(validationErrors []string) string {
	if len(validationErrors) == 0 {
		return ""
	}
	return "Validation errors: " + strings.Join(validationErrors, "; ")
}
