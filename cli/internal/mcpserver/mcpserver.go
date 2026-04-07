// Package mcpserver implements the cordon stdio MCP server.
// It is started via `cordon --mcp` and exposes MCP tools that agents can call
// to interact with Cordon policy (request access passes, etc.).
package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Run starts the stdio MCP server and blocks until the client disconnects.
// It resolves the repo root from the process working directory, then registers
// the cordon_request_access tool and begins serving JSON-RPC over stdio.
func Run(_ context.Context) error {
	root, _, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("mcp: find repo root: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("mcp: resolve repo root: %w", err)
	}

	s := server.NewMCPServer("cordon", "0.1.0",
		server.WithToolCapabilities(false),
		server.WithElicitation(),
	)

	requestAccessTool := mcp.NewTool("cordon_request_access",
		mcp.WithDescription(
			"Request temporary write access to a target protected by a Cordon policy rule. "+
				"Call this tool when a Cordon hook has denied a write operation. "+
				"The user will be asked to approve or deny the request. "+
				"Returns a pass ID and expiry time on success.",
		),
		mcp.WithString("rule_str",
			mcp.Required(),
			mcp.Description("Rule target string from the deny reason (file path/pattern or command string)."),
		),
		mcp.WithString("file_path",
			mcp.Description("Deprecated compatibility field. Use rule_str instead."),
		),
		mcp.WithString("reason",
			mcp.Description("Brief explanation of why write access is needed (optional)."),
		),
	)

	s.AddTool(requestAccessTool, makeRequestAccessHandler(s, absRoot))

	return server.ServeStdio(s)
}

// makeRequestAccessHandler returns a ToolHandlerFunc that elicits user
// confirmation before issuing a 60-minute pass for the requested file.
func makeRequestAccessHandler(s *server.MCPServer, absRoot string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := req.RequireString("rule_str")
		if err != nil {
			// Backward compatibility for older clients.
			legacy, legacyErr := req.RequireString("file_path")
			if legacyErr != nil {
				return nil, fmt.Errorf("rule_str is required")
			}
			target = legacy
		}

		reason, _ := req.RequireString("reason")

		// Normalize to canonical repo-relative form when possible.
		filePath := store.NormalizeFilePath(target, absRoot)

		// Open the policy database and look up the covering file/command rule.
		policyDB, err := store.OpenPolicyDB(absRoot)
		if err != nil {
			return nil, fmt.Errorf("cordon: open policy database: %w", err)
		}
		defer policyDB.Close()

		if err := store.MigratePolicyDB(policyDB); err != nil {
			return nil, fmt.Errorf("cordon: migrate policy database: %w", err)
		}

		fileRule, err := store.FileRuleForPath(policyDB, filePath, absRoot)
		if err != nil {
			return nil, fmt.Errorf("cordon: file rule lookup: %w", err)
		}
		var commandRule *store.CommandRule
		if fileRule == nil {
			commandRule, err = store.MatchCommandRule(policyDB, target)
			if err != nil {
				return nil, fmt.Errorf("cordon: command rule lookup: %w", err)
			}
		}
		if fileRule == nil && commandRule == nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("%q is not covered by any Cordon file rule or command rule — no pass can be issued.", target),
			), nil
		}

		ruleType := "file"
		rulePattern := ""
		ruleID := ""
		passFilePath := filePath
		if fileRule != nil {
			rulePattern = fileRule.Pattern
			ruleID = fileRule.ID
		} else {
			ruleType = "command"
			rulePattern = commandRule.Pattern
			ruleID = commandRule.ID
			passFilePath = ""
		}

		// Ask the user for confirmation via elicitation.
		msg := fmt.Sprintf(
			"Your agent is requesting read/write access to a target protected by a Cordon policy.\n\nTarget: %s\nRule Type: %s\nRule: %s",
			target, ruleType, rulePattern,
		)
		if reason != "" {
			msg += fmt.Sprintf("\nAgent's Reason: %s", reason)
		}
		msg += "\n\nDo you want to grant your agent a 60-minute access pass?"

		elicitResult, err := s.RequestElicitation(ctx, mcp.ElicitationRequest{
			Params: mcp.ElicitationParams{
				Message: msg,
				RequestedSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"Pass Approved": map[string]interface{}{
							"type":        "boolean",
							"description": "Grant write access?",
							"default":     false,
						},
					},
					"required": []string{"Pass Approved"},
				},
			},
		})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("Could not request user confirmation: %v", err),
			), nil
		}

		// Check whether the user approved.
		if elicitResult.Action != mcp.ElicitationResponseActionAccept {
			return mcp.NewToolResultText(
				fmt.Sprintf("Access request for %s was declined by the user.", filePath),
			), nil
		}

		approved := false
		if content, ok := elicitResult.Content.(map[string]interface{}); ok {
			if v, ok := content["Pass Approved"].(bool); ok {
				approved = v
			}
		}
		if !approved {
			return mcp.NewToolResultText(
				fmt.Sprintf("Access request for %s was denied by the user.", filePath),
			), nil
		}

		// User approved — issue the pass.
		const defaultMinutes = 60
		expiresAt := time.Now().Add(defaultMinutes * time.Minute)
		expiresAtStr := expiresAt.UTC().Format(time.RFC3339)
		now := time.Now().UTC().Format(time.RFC3339)
		dur := defaultMinutes

		dataDB, err := store.OpenDataDB(absRoot)
		if err != nil {
			return nil, fmt.Errorf("cordon: open data database: %w", err)
		}
		defer dataDB.Close()

		if err := store.MigrateDataDB(dataDB); err != nil {
			return nil, fmt.Errorf("cordon: migrate data database: %w", err)
		}

		p := store.Pass{
			FileRuleID:      ruleID,
			Pattern:         rulePattern,
			FilePath:        passFilePath,
			IssuedTo:        "agent",
			IssuedBy:        store.CurrentOSUser(),
			Status:          "active",
			DurationMinutes: &dur,
			IssuedAt:        now,
			ExpiresAt:       expiresAtStr,
		}

		if err := store.IssuePass(dataDB, p); err != nil {
			return nil, fmt.Errorf("cordon: issue pass: %w", err)
		}

		// Reload to obtain the generated ID (IssuePass assigns it internally).
		passes, err := store.ListPasses(dataDB)
		if err != nil {
			return nil, fmt.Errorf("cordon: reload pass: %w", err)
		}
		var issued store.Pass
		for _, lp := range passes {
			if lp.Pattern == rulePattern && lp.FilePath == passFilePath && lp.IssuedAt == now {
				issued = lp
				break
			}
		}

		// Audit log — failures are non-fatal.
		_ = store.InsertAudit(dataDB, store.AuditEntry{
			EventType:  "pass_issue",
			FilePath:   target,
			FileRuleID: ruleID,
			PassID:     issued.ID,
			User:       store.CurrentOSUser(),
			Agent:      "mcp",
			Detail:     fmt.Sprintf("source=mcp_request_access duration=%dm expires_at=%s", defaultMinutes, expiresAtStr),
		})

		result := fmt.Sprintf(
			"Access granted for %s\nPass ID:    %s\nExpires:    %s\nRule Type:  %s\nRule:       %s",
			target, issued.ID, expiresAtStr, ruleType, rulePattern,
		)
		return mcp.NewToolResultText(result), nil
	}
}
