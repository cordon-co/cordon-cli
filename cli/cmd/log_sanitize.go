package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
)

const repoDirPlaceholder = "/<REPO_PATH>"

// sanitizeRepoPathInString best-effort replaces absolute repo root prefixes
// with /<REPO_DIR> in arbitrary strings.
func sanitizeRepoPathInString(input, absRoot string) string {
	if input == "" || absRoot == "" {
		return input
	}

	root := filepath.Clean(absRoot)
	if root == "." || root == "/" || root == `\` {
		return input
	}

	candidates := uniqueStrings([]string{
		root,
		filepath.ToSlash(root),
		filepath.FromSlash(filepath.ToSlash(root)),
	})

	out := input
	for _, c := range candidates {
		if c == "" {
			continue
		}
		out = strings.ReplaceAll(out, c, repoDirPlaceholder)
	}
	return out
}

// sanitizeRepoPathInJSONStrings best-effort redacts absolute repo root paths in
// string values of a JSON blob. If parsing fails, it falls back to plain string
// replacement and returns that result.
func sanitizeRepoPathInJSONStrings(raw, absRoot string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return sanitizeRepoPathInString(raw, absRoot)
	}

	payload = sanitizeJSONValue(payload, absRoot)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(payload)
	if err != nil {
		return sanitizeRepoPathInString(raw, absRoot)
	}
	return strings.TrimSpace(buf.String())
}

func sanitizeJSONValue(v any, absRoot string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			x[k] = sanitizeJSONValue(child, absRoot)
		}
		return x
	case []any:
		for i := range x {
			x[i] = sanitizeJSONValue(x[i], absRoot)
		}
		return x
	case string:
		return sanitizeRepoPathInString(x, absRoot)
	default:
		return v
	}
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
