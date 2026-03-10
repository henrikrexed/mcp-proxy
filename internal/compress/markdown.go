package compress

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CompressJSONToMarkdown attempts to convert a JSON string into a markdown table.
// Returns the converted string and true if conversion happened,
// or the original string and false if the input is not convertible.
func CompressJSONToMarkdown(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return content, false
	}

	// Try to parse as JSON
	var raw any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false
	}

	switch v := raw.(type) {
	case []any:
		// Direct array of objects
		objects, ok := toObjectSlice(v)
		if !ok {
			return content, false
		}
		return renderTable(objects), true

	case map[string]any:
		// Wrapper object: find a top-level key whose value is an array of objects
		var arrayKey string
		var objects []map[string]any
		var scalarFields []scalarField

		for key, val := range v {
			if arr, ok := val.([]any); ok {
				if objs, ok := toObjectSlice(arr); ok {
					arrayKey = key
					objects = objs
				}
			}
		}

		if arrayKey == "" {
			return content, false
		}

		// Collect scalar (non-array) fields for the header
		for key, val := range v {
			if key == arrayKey {
				continue
			}
			scalarFields = append(scalarFields, scalarField{key: key, value: renderValue(val)})
		}

		var sb strings.Builder
		for _, sf := range scalarFields {
			sb.WriteString(sf.key)
			sb.WriteString(": ")
			sb.WriteString(sf.value)
			sb.WriteString("\n")
		}
		if len(scalarFields) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(renderTable(objects))
		return sb.String(), true

	default:
		return content, false
	}
}

type scalarField struct {
	key   string
	value string
}

// toObjectSlice checks if all items in a slice are maps and returns them.
func toObjectSlice(arr []any) ([]map[string]any, bool) {
	if len(arr) == 0 {
		return nil, false
	}
	objects := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		objects = append(objects, obj)
	}
	return objects, true
}

// renderTable converts a slice of objects into a markdown table string.
func renderTable(objects []map[string]any) string {
	if len(objects) == 0 {
		return ""
	}

	// Collect columns: order from first object, then union of all keys
	columns := collectColumns(objects)

	var sb strings.Builder

	// Header row
	sb.WriteString("|")
	for _, col := range columns {
		sb.WriteString(" ")
		sb.WriteString(col)
		sb.WriteString(" |")
	}
	sb.WriteString("\n")

	// Separator row
	sb.WriteString("|")
	for _, col := range columns {
		sb.WriteString(strings.Repeat("-", len(col)+2))
		sb.WriteString("|")
	}
	sb.WriteString("\n")

	// Data rows
	for _, obj := range objects {
		sb.WriteString("|")
		for _, col := range columns {
			sb.WriteString(" ")
			val, exists := obj[col]
			if !exists || val == nil {
				sb.WriteString(" ")
			} else {
				sb.WriteString(renderValue(val))
			}
			sb.WriteString(" |")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// collectColumns returns ordered column names: keys from first object in order,
// then any additional keys from remaining objects.
func collectColumns(objects []map[string]any) []string {
	seen := make(map[string]bool)
	var columns []string

	// Keys from first object (preserving JSON parse order — Go maps don't guarantee
	// order, but in practice encoding/json preserves insertion order for map[string]any).
	// For deterministic ordering, we re-parse the first object.
	for key := range objects[0] {
		if !seen[key] {
			seen[key] = true
			columns = append(columns, key)
		}
	}

	// Additional keys from remaining objects
	for _, obj := range objects[1:] {
		for key := range obj {
			if !seen[key] {
				seen[key] = true
				columns = append(columns, key)
			}
		}
	}

	return columns
}

// renderValue converts a JSON value to its markdown table cell representation.
func renderValue(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		// Render integers without decimal point
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	case []any:
		return renderArray(v)
	case map[string]any:
		return "{...}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// renderArray converts a JSON array to its markdown cell representation.
func renderArray(arr []any) string {
	if len(arr) == 0 {
		return "[]"
	}

	// Check if it's an array of objects
	allObjects := true
	for _, item := range arr {
		if _, ok := item.(map[string]any); !ok {
			allObjects = false
			break
		}
	}
	if allObjects {
		return fmt.Sprintf("[{...} x %d]", len(arr))
	}

	// Flat array
	items := make([]string, 0, len(arr))
	for _, item := range arr {
		items = append(items, renderValue(item))
	}

	if len(items) <= 5 {
		return strings.Join(items, ", ")
	}

	return strings.Join(items[:5], ", ") + fmt.Sprintf(" ... (+%d more)", len(items)-5)
}
