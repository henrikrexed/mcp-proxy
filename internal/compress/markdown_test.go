package compress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompressJSONToMarkdown_FlatArray(t *testing.T) {
	input := `[
		{"name": "svc-a", "namespace": "default", "type": "ClusterIP"},
		{"name": "svc-b", "namespace": "kube-system", "type": "NodePort"}
	]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "| name ") {
		t.Errorf("expected header with 'name', got:\n%s", result)
	}
	if !strings.Contains(result, "| svc-a ") {
		t.Errorf("expected row with 'svc-a', got:\n%s", result)
	}
	if !strings.Contains(result, "| svc-b ") {
		t.Errorf("expected row with 'svc-b', got:\n%s", result)
	}

	// Verify it's a proper markdown table (header + separator + 2 data rows)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines (header + separator + 2 rows), got %d:\n%s", len(lines), result)
	}
}

func TestCompressJSONToMarkdown_WrapperObject(t *testing.T) {
	input := `{
		"count": 2,
		"collectors": [
			{"name": "col-a", "namespace": "default"},
			{"name": "col-b", "namespace": "monitoring"}
		]
	}`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "count: 2") {
		t.Errorf("expected scalar header 'count: 2', got:\n%s", result)
	}
	if !strings.Contains(result, "| name ") {
		t.Errorf("expected table with 'name' column, got:\n%s", result)
	}
	if !strings.Contains(result, "| col-a ") {
		t.Errorf("expected row with 'col-a', got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_NestedObjects(t *testing.T) {
	input := `[
		{"name": "pod-a", "metadata": {"labels": {"app": "web"}}}
	]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "{...}") {
		t.Errorf("expected nested object rendered as '{...}', got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_ArrayValues(t *testing.T) {
	input := `[
		{"name": "svc-a", "ports": [8080, 443]},
		{"name": "svc-b", "ports": [53]}
	]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "8080, 443") {
		t.Errorf("expected flat array rendered as '8080, 443', got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_LargeArray(t *testing.T) {
	input := `[{"name": "a", "tags": [1, 2, 3, 4, 5, 6, 7]}]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "... (+2 more)") {
		t.Errorf("expected truncated array with '... (+2 more)', got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_ArrayOfObjects(t *testing.T) {
	input := `[{"name": "a", "items": [{"x": 1}, {"x": 2}, {"x": 3}]}]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "[{...} x 3]") {
		t.Errorf("expected array of objects rendered as '[{...} x 3]', got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_NonConvertibleScalar(t *testing.T) {
	input := `"just a string"`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for scalar string")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_NonConvertibleNumber(t *testing.T) {
	input := `42`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for scalar number")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_InvalidJSON(t *testing.T) {
	input := `{not valid json`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for invalid JSON")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_EmptyString(t *testing.T) {
	result, converted := CompressJSONToMarkdown("")
	if converted {
		t.Fatal("expected no conversion for empty string")
	}
	if result != "" {
		t.Errorf("expected empty string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_EmptyArray(t *testing.T) {
	input := `[]`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for empty array")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_ArrayOfNonObjects(t *testing.T) {
	input := `[1, 2, 3]`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for array of non-objects")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_MixedKeys(t *testing.T) {
	input := `[
		{"name": "a", "cpu": "100m"},
		{"name": "b", "memory": "256Mi"},
		{"name": "c", "cpu": "200m", "memory": "512Mi"}
	]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	// Should have all three columns: name, cpu, memory
	if !strings.Contains(result, "name") || !strings.Contains(result, "cpu") || !strings.Contains(result, "memory") {
		t.Errorf("expected all keys as columns, got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_BooleanAndNull(t *testing.T) {
	input := `[{"name": "a", "ready": true, "deleted": null}]`
	result, converted := CompressJSONToMarkdown(input)
	if !converted {
		t.Fatal("expected conversion to happen")
	}
	if !strings.Contains(result, "true") {
		t.Errorf("expected 'true' for boolean, got:\n%s", result)
	}
}

func TestCompressJSONToMarkdown_LargeDataset(t *testing.T) {
	// Generate 100+ items
	items := make([]map[string]any, 150)
	for i := range items {
		items[i] = map[string]any{
			"name":      strings.Repeat("x", 10),
			"namespace": "default",
			"index":     float64(i),
		}
	}
	data, _ := json.Marshal(items)
	result, converted := CompressJSONToMarkdown(string(data))
	if !converted {
		t.Fatal("expected conversion for large dataset")
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	// header + separator + 150 data rows = 152
	if len(lines) != 152 {
		t.Errorf("expected 152 lines, got %d", len(lines))
	}
}

func TestCompressJSONToMarkdown_ObjectWithoutArray(t *testing.T) {
	input := `{"name": "foo", "count": 42}`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for object without array value")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}

func TestCompressJSONToMarkdown_WrapperWithScalarArrayOnly(t *testing.T) {
	// Object where the only array is not an array of objects
	input := `{"name": "foo", "tags": [1, 2, 3]}`
	result, converted := CompressJSONToMarkdown(input)
	if converted {
		t.Fatal("expected no conversion for object with non-object array")
	}
	if result != input {
		t.Errorf("expected original string returned, got: %s", result)
	}
}
