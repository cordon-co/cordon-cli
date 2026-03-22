package agents

import (
	"encoding/json"
	"testing"
)

func TestStripJSONCPreservesURLsAndRemovesComments(t *testing.T) {
	raw := []byte(`{
  "$schema": "https://opencode.ai/config.json", // schema URL
  "mcp": {
    "cordon": {
      "type": "local",
      "command": ["cordon", "--mcp",],
    },
  },
}`)

	clean := stripJSONC(raw)
	var parsed map[string]interface{}
	if err := json.Unmarshal(clean, &parsed); err != nil {
		t.Fatalf("expected valid JSON after stripping JSONC, got error: %v\ncleaned:\n%s", err, string(clean))
	}

	schema, ok := parsed["$schema"].(string)
	if !ok {
		t.Fatalf("expected $schema string, got %#v", parsed["$schema"])
	}
	if schema != "https://opencode.ai/config.json" {
		t.Fatalf("unexpected schema value: %q", schema)
	}
}

func TestStripJSONCHandlesBlockComments(t *testing.T) {
	raw := []byte(`{
  "mcp": {
    /* plugin entries */
    "cordon": {
      "type": "local",
      "command": ["cordon", "--mcp"],
    },
  },
}`)

	clean := stripJSONC(raw)
	var parsed map[string]interface{}
	if err := json.Unmarshal(clean, &parsed); err != nil {
		t.Fatalf("expected valid JSON after stripping JSONC with block comments, got error: %v\ncleaned:\n%s", err, string(clean))
	}
}
