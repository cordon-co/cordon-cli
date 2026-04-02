package secrets

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/zricethezav/gitleaks/v8/detect"
)

// ScanResult is the secret detection output for a tool_input payload.
type ScanResult struct {
	RedactedToolInput json.RawMessage
	RuleIDs           []string
	Redactions        []Redaction
}

// Redaction is a single secret replacement mapping.
type Redaction struct {
	Secret string
	RuleID string
}

// Scanner detects and redacts secrets in tool_input values.
type Scanner struct {
	detector *detect.Detector
}

// NewScanner creates a scanner using gitleaks default config.
func NewScanner() (*Scanner, error) {
	d, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return nil, err
	}
	return &Scanner{detector: d}, nil
}

// ScanAndRedact scans string fields in tool_input and replaces detected
// secret values with <SECRET:<rule_id>> placeholders.
func (s *Scanner) ScanAndRedact(toolInput json.RawMessage) (ScanResult, error) {
	result := ScanResult{RedactedToolInput: toolInput, RuleIDs: []string{}, Redactions: []Redaction{}}
	trimmed := strings.TrimSpace(string(toolInput))
	if trimmed == "" {
		return result, nil
	}

	var payload any
	if err := json.Unmarshal(toolInput, &payload); err != nil {
		// Keep original payload on malformed JSON.
		return result, nil
	}

	acc := redactAccum{
		rules:        map[string]struct{}{},
		replacements: map[string]string{},
	}
	payload = s.redactValue(payload, &acc)

	if len(acc.rules) == 0 {
		return result, nil
	}

	ids := make([]string, 0, len(acc.rules))
	for id := range acc.rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result.RuleIDs = ids
	for secret, ruleID := range acc.replacements {
		result.Redactions = append(result.Redactions, Redaction{
			Secret: secret,
			RuleID: ruleID,
		})
	}
	sort.Slice(result.Redactions, func(i, j int) bool {
		if result.Redactions[i].RuleID == result.Redactions[j].RuleID {
			return result.Redactions[i].Secret < result.Redactions[j].Secret
		}
		return result.Redactions[i].RuleID < result.Redactions[j].RuleID
	})

	redacted, err := json.Marshal(payload)
	if err != nil {
		// Fail open and keep original payload.
		return result, nil
	}
	result.RedactedToolInput = redacted
	return result, nil
}

type redactAccum struct {
	rules        map[string]struct{}
	replacements map[string]string // secret -> ruleID
}

func (s *Scanner) redactValue(v any, acc *redactAccum) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			x[k] = s.redactValue(child, acc)
		}
		return x
	case []any:
		for i := range x {
			x[i] = s.redactValue(x[i], acc)
		}
		return x
	case string:
		return s.redactString(x, acc)
	default:
		return v
	}
}

func (s *Scanner) redactString(input string, acc *redactAccum) string {
	findings := s.detector.DetectString(input)
	if len(findings) == 0 {
		return input
	}

	redacted := input
	for _, finding := range findings {
		ruleID := sanitizeRuleID(finding.RuleID)
		acc.rules[ruleID] = struct{}{}
		secret := finding.Secret
		if secret == "" {
			secret = finding.Match
		}
		if secret == "" {
			continue
		}
		if _, exists := acc.replacements[secret]; !exists {
			acc.replacements[secret] = ruleID
		}
		redacted = strings.ReplaceAll(redacted, secret, "<SECRET:"+ruleID+">")
	}
	return redacted
}

func sanitizeRuleID(ruleID string) string {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return "unknown"
	}
	return ruleID
}
