package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FieldRule defines validation rules for a single field
type FieldRule struct {
	Type     string `yaml:"type" json:"type"`         // "string", "number", "bool", "array"
	Required bool   `yaml:"required" json:"required"` // Field must be present
	MinLen   int    `yaml:"min_len" json:"min_len"`   // Minimum string length
	MaxLen   int    `yaml:"max_len" json:"max_len"`   // Maximum string length
	Min      *float64 `yaml:"min" json:"min"`         // Minimum number value
	Max      *float64 `yaml:"max" json:"max"`         // Maximum number value
}

// ValidationSchema defines the expected structure of a request body
type ValidationSchema struct {
	Fields map[string]FieldRule `yaml:"fields" json:"fields"`
}

// PathValidation maps method+path to a validation schema
type RequestValidator struct {
	schemas map[string]*ValidationSchema // key: "POST /api/users"
}

func NewRequestValidator(schemas map[string]*ValidationSchema) *RequestValidator {
	return &RequestValidator{schemas: schemas}
}

func (v *RequestValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate POST, PUT, PATCH
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		// Find matching schema
		key := r.Method + " " + r.URL.Path
		schema := v.findSchema(key)
		if schema == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Read body
		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			writeValidationError(w, []string{"failed to read request body"})
			return
		}

		// Restore body for downstream handlers
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Parse JSON
		var body map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			writeValidationError(w, []string{"invalid JSON body"})
			return
		}

		// Validate
		errors := v.validate(body, schema)
		if len(errors) > 0 {
			writeValidationError(w, errors)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (v *RequestValidator) findSchema(key string) *ValidationSchema {
	// Exact match first
	if schema, ok := v.schemas[key]; ok {
		return schema
	}

	// Prefix match (e.g., "POST /api/users" matches "POST /api/users/123")
	for pattern, schema := range v.schemas {
		parts := strings.SplitN(pattern, " ", 2)
		keyParts := strings.SplitN(key, " ", 2)
		if len(parts) == 2 && len(keyParts) == 2 {
			if parts[0] == keyParts[0] && strings.HasPrefix(keyParts[1], parts[1]) {
				return schema
			}
		}
	}

	return nil
}

func (v *RequestValidator) validate(body map[string]interface{}, schema *ValidationSchema) []string {
	var errors []string

	for fieldName, rule := range schema.Fields {
		value, exists := body[fieldName]

		if rule.Required && !exists {
			errors = append(errors, fmt.Sprintf("field '%s' is required", fieldName))
			continue
		}

		if !exists {
			continue
		}

		switch rule.Type {
		case "string":
			str, ok := value.(string)
			if !ok {
				errors = append(errors, fmt.Sprintf("field '%s' must be a string", fieldName))
				continue
			}
			if rule.MinLen > 0 && len(str) < rule.MinLen {
				errors = append(errors, fmt.Sprintf("field '%s' must be at least %d characters", fieldName, rule.MinLen))
			}
			if rule.MaxLen > 0 && len(str) > rule.MaxLen {
				errors = append(errors, fmt.Sprintf("field '%s' must be at most %d characters", fieldName, rule.MaxLen))
			}

		case "number":
			num, ok := value.(float64)
			if !ok {
				errors = append(errors, fmt.Sprintf("field '%s' must be a number", fieldName))
				continue
			}
			if rule.Min != nil && num < *rule.Min {
				errors = append(errors, fmt.Sprintf("field '%s' must be at least %v", fieldName, *rule.Min))
			}
			if rule.Max != nil && num > *rule.Max {
				errors = append(errors, fmt.Sprintf("field '%s' must be at most %v", fieldName, *rule.Max))
			}

		case "bool":
			if _, ok := value.(bool); !ok {
				errors = append(errors, fmt.Sprintf("field '%s' must be a boolean", fieldName))
			}

		case "array":
			if _, ok := value.([]interface{}); !ok {
				errors = append(errors, fmt.Sprintf("field '%s' must be an array", fieldName))
			}
		}
	}

	return errors
}

func writeValidationError(w http.ResponseWriter, errors []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "validation failed",
		"details": errors,
	})
}
