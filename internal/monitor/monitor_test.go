package monitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewContractMonitor(t *testing.T) {
	// Create a dummy schema file for testing
	testSchemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"title": "TestSchema",
		"type": "object",
		"properties": { "name": { "type": "string" } },
		"required": ["name"]
	}`
	schemaDir := t.TempDir()
	schemaFile := filepath.Join(schemaDir, "test_schema.json")
	if err := os.WriteFile(schemaFile, []byte(testSchemaContent), 0644); err != nil {
		t.Fatalf("Failed to write test schema file: %v", err)
	}

	t.Run("SuccessfulLoad", func(t *testing.T) {
		cm, err := NewContractMonitor(schemaFile)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cm == nil {
			t.Fatal("Expected ContractMonitor instance, got nil")
		}
		if cm.schemaLoader == nil {
			t.Fatal("Expected schemaLoader to be initialized, got nil")
		}
	})

	t.Run("SchemaFileNotFound", func(t *testing.T) {
		_, err := NewContractMonitor("non_existent_schema.json")
		if err == nil {
			t.Fatal("Expected error for non-existent schema, got nil")
		}
		// Check for the actual error message parts from gojsonschema
		expectedErrorPart1 := "error loading or compiling schema"
		expectedErrorPart2 := "must be canonical" // This is part of the specific error for non-existent files
		if !strings.Contains(err.Error(), expectedErrorPart1) || !strings.Contains(err.Error(), expectedErrorPart2) {
			t.Errorf("Expected error to contain '%s' and '%s', got: %v", expectedErrorPart1, expectedErrorPart2, err)
		}
	})

	t.Run("InvalidSchemaSyntax", func(t *testing.T) {
		invalidSchemaFile := filepath.Join(schemaDir, "invalid_schema.json")
		if err := os.WriteFile(invalidSchemaFile, []byte("{invalid_json"), 0644); err != nil {
			t.Fatalf("Failed to write invalid test schema file: %v", err)
		}
		_, err := NewContractMonitor(invalidSchemaFile)
		if err == nil {
			t.Fatal("Expected error for invalid schema syntax, got nil")
		}
	})
}

func TestContractMonitor_Validate(t *testing.T) {
	// Use the testdata schema for these tests
	// Construct path relative to current file to find testdata
	// Note: This assumes 'go test' is run from the package directory or project root in a way that relative paths work.
	// A more robust way in general might be to use runtime.Caller to get current file's dir.
	// For now, direct relative path from where `go test` is typically run.
	schemaPath := filepath.Join("testdata", "test_schema.json")

	// Ensure the testdata directory and schema file exist
	// The test_schema.json should have been created by a previous step or checked into VCS.
	// This check is more for safety during test execution if testdata is gitignored or built dynamically.
	// If the file `internal/monitor/testdata/test_schema.json` is guaranteed by a previous step,
	// then explicit creation here might be redundant or could be simplified.
	// For this specific subtask, the file is created in step 2.
	// So, we can assume it exists. If not, NewContractMonitor will fail, which is part of the test.

	cm, err := NewContractMonitor(schemaPath)
	if err != nil {
		// This could fail if test_schema.json wasn't created in the previous step as expected.
		// Or if the path `testdata/test_schema.json` is not correct relative to test execution dir.
		t.Fatalf("Failed to create ContractMonitor for validation tests (schema: %s): %v. Ensure testdata/test_schema.json exists.", schemaPath, err)
	}

	tests := []struct {
		name          string
		payload       string
		expectValid   bool
		expectErrors  bool // True if either validation errors OR functional error from Validate() is expected
		errorContains []string
	}{
		{
			name:        "ValidPayload",
			payload:     `{"name": "John Doe", "age": 30, "email": "john.doe@example.com"}`,
			expectValid: true,
		},
		{
			name:          "InvalidPayload_MissingRequiredField",
			payload:       `{"name": "Jane Doe"}`, // age is missing
			expectValid:   false,
			expectErrors:  true,
			errorContains: []string{"age", "(root): age is required"}, // More specific error text
		},
		{
			name:          "InvalidPayload_WrongType",
			payload:       `{"name": "Test", "age": "thirty"}`, // age should be integer
			expectValid:   false,
			expectErrors:  true,
			errorContains: []string{"age", "Invalid type. Expected: integer, given: string"},
		},
		{
			name:          "InvalidPayload_FormatViolation",
			payload:       `{"name": "Test", "age": 25, "email": "not-an-email"}`,
			expectValid:   false,
			expectErrors:  true,
			errorContains: []string{"email", "Does not match format 'email'"},
		},
		{
			name:        "InvalidPayload_AdditionalPropertyAllowed", // Corrected expectation
			payload:     `{"name": "Test", "age": 25, "city": "New York"}`,
			expectValid: true, // Schema allows additional properties by default
		},
		{
			name:         "MalformedJSON",
			payload:      `{"name": "Test", "age": 25,`,
			expectValid:  false,
			expectErrors: true, // Error comes from gojsonschema.Validate's internal JSON parsing
			// errorContains: []string{"unexpected EOF"}, // Error message might vary
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, validationErrs, funcErr := cm.Validate([]byte(tt.payload))

			if tt.expectErrors {
				if funcErr == nil && len(validationErrs) == 0 {
					t.Errorf("Expected errors, but got none (funcErr: %v, validationErrs: %v)", funcErr, validationErrs)
				}
			} else { // Expect no errors at all
				if funcErr != nil {
					t.Errorf("Expected no functional error, got %v", funcErr)
				}
				if len(validationErrs) > 0 {
					t.Errorf("Expected no validation errors, got %v", validationErrs)
				}
			}

			if valid != tt.expectValid {
				t.Errorf("Expected valid=%v, got valid=%v. ValidationErrors: %v, FuncErr: %v", tt.expectValid, valid, validationErrs, funcErr)
			}

			if len(tt.errorContains) > 0 {
				combinedErrors := ""
				if len(validationErrs) > 0 {
					combinedErrors = strings.Join(validationErrs, "; ")
				}
				if funcErr != nil { // include Validate's own error message if any
					if combinedErrors != "" {
						combinedErrors += "; "
					}
					combinedErrors += funcErr.Error()
				}

				for _, ec := range tt.errorContains {
					if !strings.Contains(combinedErrors, ec) {
						t.Errorf("Expected errors to contain '%s', but got: %s", ec, combinedErrors)
					}
				}
			}
		})
	}
}

func TestFormatErrors(t *testing.T) {
	tests := []struct {
		name           string
		errors         []string
		expectedOutput string
	}{
		{
			name:           "NoErrors",
			errors:         []string{},
			expectedOutput: "",
		},
		{
			name:           "SingleError",
			errors:         []string{"Error: Field 'X' is required."},
			expectedOutput: "Validation errors: Error: Field 'X' is required.",
		},
		{
			name:           "MultipleErrors",
			errors:         []string{"Error 1", "Error 2"},
			expectedOutput: "Validation errors: Error 1; Error 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatErrors(tt.errors)
			if output != tt.expectedOutput {
				t.Errorf("Expected '%s', got '%s'", tt.expectedOutput, output)
			}
		})
	}
}
