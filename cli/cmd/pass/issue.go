package pass

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var issueFile string
var issueDuration string

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Issue a temporary access pass",
	Long: `Issue a temporary pass granting an agent write access to a protected file.

The file must already be covered by a Cordon file rule. Duration formats:
  60m        60 minutes
  24h        24 hours
  7d         7 days
  1w         1 week
  indefinite no expiry`,
	Args: cobra.NoArgs,
	RunE: runPassIssue,
}

func init() {
	issueCmd.Flags().StringVar(&issueFile, "file", "", "File path to grant access to (required)")
	issueCmd.Flags().StringVar(&issueDuration, "duration", "60m", "Duration (e.g. 60m, 24h, 7d, 1w, indefinite)")
	_ = issueCmd.MarkFlagRequired("file")
}

type passIssueResult struct {
	Pass store.Pass `json:"pass"`
}

func runPassIssue(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("pass issue: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("pass issue: resolve repo root: %w", err)
	}

	// Normalize the file path to repo-relative (consistent with how file rules are stored).
	issueFile = store.NormalizePattern(issueFile, absRoot)

	// Validate the file is covered by a file rule.
	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("pass issue: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("pass issue: migrate policy database: %w", err)
	}

	rule, err := store.FileRuleForPath(policyDB, issueFile, absRoot)
	if err != nil {
		return fmt.Errorf("pass issue: file rule lookup: %w", err)
	}
	if rule == nil {
		return fmt.Errorf("pass issue: %q is not covered by any file rule — add one first with: cordon file add <pattern>", issueFile)
	}

	// Parse the requested duration.
	durationMinutes, expiresAt, err := parseDuration(issueDuration)
	if err != nil {
		return fmt.Errorf("pass issue: %w", err)
	}

	user := store.CurrentOSUser()
	now := time.Now().UTC().Format(time.RFC3339)
	expiresAtStr := ""
	if expiresAt != nil {
		expiresAtStr = expiresAt.UTC().Format(time.RFC3339)
	}

	p := store.Pass{
		FileRuleID:      rule.ID,
		Pattern:         rule.Pattern,
		FilePath:        issueFile,
		IssuedTo:        user,
		IssuedBy:        user,
		Status:          "active",
		DurationMinutes: durationMinutes,
		IssuedAt:        now,
		ExpiresAt:       expiresAtStr,
	}

	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return fmt.Errorf("pass issue: open data database: %w", err)
	}
	defer dataDB.Close()

	if err := store.MigrateDataDB(dataDB); err != nil {
		return fmt.Errorf("pass issue: migrate data database: %w", err)
	}

	if err := store.IssuePass(dataDB, p); err != nil {
		return fmt.Errorf("pass issue: %w", err)
	}

	// Reload pass to get the generated ID.
	passes, err := store.ListPasses(dataDB)
	if err != nil {
		return fmt.Errorf("pass issue: reload pass: %w", err)
	}
	// The most recently issued pass for this file is first (ListPasses is DESC).
	var issued store.Pass
	for _, lp := range passes {
		if lp.FilePath == issueFile && lp.IssuedAt == now {
			issued = lp
			break
		}
	}

	// Audit log.
	_ = store.InsertAudit(dataDB, store.AuditEntry{
		EventType:  "pass_issue",
		FilePath:   issueFile,
		FileRuleID: rule.ID,
		PassID:     issued.ID,
		User:       user,
		Detail:     fmt.Sprintf("duration=%s expires_at=%s", issueDuration, expiresAtStr),
	})

	if flags.JSON {
		out, _ := json.MarshalIndent(passIssueResult{Pass: issued}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	expiry := "never"
	if expiresAt != nil {
		expiry = expiresAt.Local().Format("2006-01-02 15:04:05")
	}
	fmt.Printf("pass issued for %s\n", issueFile)
	fmt.Printf("  id:        %s\n", issued.ID)
	fmt.Printf("  file rule: %s\n", rule.Pattern)
	fmt.Printf("  expires:   %s\n", expiry)
	return nil
}

// parseDuration parses a human-friendly duration string into the number of
// minutes and an expiry time. Supported formats: 60m, 24h, 7d, 1w, indefinite.
// Returns (nil, nil, nil) for indefinite passes.
func parseDuration(s string) (*int, *time.Time, error) {
	if s == "indefinite" || s == "" {
		return nil, nil, nil
	}

	if len(s) < 2 {
		return nil, nil, fmt.Errorf("invalid duration %q: use 60m, 24h, 7d, 1w, or indefinite", s)
	}

	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil || n <= 0 {
		return nil, nil, fmt.Errorf("invalid duration %q: use 60m, 24h, 7d, 1w, or indefinite", s)
	}

	var minutes int
	switch unit {
	case 'm':
		minutes = n
	case 'h':
		minutes = n * 60
	case 'd':
		minutes = n * 24 * 60
	case 'w':
		minutes = n * 7 * 24 * 60
	default:
		return nil, nil, fmt.Errorf("invalid duration unit %q in %q: use m, h, d, or w", string(unit), s)
	}

	expires := time.Now().Add(time.Duration(minutes) * time.Minute)
	return &minutes, &expires, nil
}
