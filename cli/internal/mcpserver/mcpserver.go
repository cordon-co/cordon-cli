// Package mcpserver implements the cordon stdio MCP server.
// It is started via `cordon --mcp` and exposes MCP tools that agents can call
// to interact with Cordon policy (request access passes, etc.).
package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
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
	)

	requestAccessTool := mcp.NewTool("cordon_request_access",
		mcp.WithDescription(
			"Request temporary write access to a file protected by a Cordon zone policy. "+
				"Call this tool when a Cordon hook has denied a write operation. "+
				"Returns a pass ID and expiry time on success.",
		),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Absolute or repo-relative path of the file access is needed for."),
		),
		mcp.WithString("reason",
			mcp.Description("Brief explanation of why write access is needed (optional)."),
		),
	)

	s.AddTool(requestAccessTool, makeRequestAccessHandler(absRoot))

	return server.ServeStdio(s)
}

// makeRequestAccessHandler returns a ToolHandlerFunc that issues a 60-minute
// pass for the requested file, provided the file is covered by a zone.
func makeRequestAccessHandler(absRoot string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawPath, err := req.RequireString("file_path")
		if err != nil {
			return nil, fmt.Errorf("file_path is required")
		}

		// Normalise to repo-relative so it matches how zone patterns are stored.
		filePath := store.NormalizePattern(rawPath, absRoot)

		// Open the policy database and look up the covering zone.
		policyDB, err := store.OpenPolicyDB(absRoot)
		if err != nil {
			return nil, fmt.Errorf("cordon: open policy database: %w", err)
		}
		defer policyDB.Close()

		if err := store.MigratePolicyDB(policyDB); err != nil {
			return nil, fmt.Errorf("cordon: migrate policy database: %w", err)
		}

		zone, err := store.ZoneForPath(policyDB, filePath, absRoot)
		if err != nil {
			return nil, fmt.Errorf("cordon: zone lookup: %w", err)
		}
		if zone == nil {
			return nil, fmt.Errorf("%q is not covered by any Cordon zone — no pass can be issued", filePath)
		}

		// Issue a 60-minute pass (self-service; elicitation flow comes later).
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
			ZoneID:          zone.ID,
			Pattern:         zone.Pattern,
			FilePath:        filePath,
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
			if lp.FilePath == filePath && lp.IssuedAt == now {
				issued = lp
				break
			}
		}

		// Audit log — failures are non-fatal.
		_ = store.InsertAudit(dataDB, store.AuditEntry{
			EventType: "pass_issue",
			FilePath:  filePath,
			ZoneID:    zone.ID,
			PassID:    issued.ID,
			User:      store.CurrentOSUser(),
			Agent:     "mcp",
			Detail:    fmt.Sprintf("source=mcp_request_access duration=%dm expires_at=%s", defaultMinutes, expiresAtStr),
		})

		result := fmt.Sprintf(
			"Access granted for %s\nPass ID:  %s\nExpires:  %s\nZone:     %s",
			filePath, issued.ID, expiresAtStr, zone.Pattern,
		)
		return mcp.NewToolResultText(result), nil
	}
}
