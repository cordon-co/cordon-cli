package agents

import (
	"encoding/json"
	"strings"
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

func TestPluginContentSupportsMultipleArgShapes(t *testing.T) {
	if !strings.Contains(pluginContent, "output?.args") {
		t.Fatal("plugin should read output?.args")
	}
	if !strings.Contains(pluginContent, "input?.args") {
		t.Fatal("plugin should fall back to input?.args")
	}
	if !strings.Contains(pluginContent, "input?.arguments") {
		t.Fatal("plugin should fall back to input?.arguments")
	}
	if !strings.Contains(pluginContent, "input?.input") {
		t.Fatal("plugin should fall back to input?.input")
	}
}

func TestPluginContentPropagatesPolicyDenials(t *testing.T) {
	if !strings.Contains(pluginContent, "CordonPolicyError") {
		t.Fatal("plugin should throw and propagate CordonPolicyError on deny")
	}
}
