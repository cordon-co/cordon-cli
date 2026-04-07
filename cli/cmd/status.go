package cmd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/agents"
	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/apicontract"
	"github.com/cordon-co/cordon-cli/cli/internal/config"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/policysync"
	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show repository, policy, auth, and agent setup status",
	Args:  cobra.NoArgs,
	RunE:  runStatus,
}

type statusOutput struct {
	Repository repositoryStatus  `json:"repository"`
	Policy     policyStatus      `json:"policy"`
	Auth       authSummary       `json:"auth"`
	Perimeter  perimeterSummary  `json:"perimeter"`
	Agents     []agentStatusLine `json:"agents"`
}

type repositoryStatus struct {
	Root        string `json:"root"`
	Initialised bool   `json:"initialised"`
	PerimeterID string `json:"perimeter_id,omitempty"`
	PolicyDB    string `json:"policy_db,omitempty"`
	DataDB      string `json:"data_db,omitempty"`
}

type policyStatus struct {
	FileRules    int `json:"file_rules"`
	CommandRules int `json:"command_rules"`
	ActivePasses int `json:"active_passes"`
}

type authSummary struct {
	Authenticated bool       `json:"authenticated"`
	AuthType      string     `json:"auth_type,omitempty"`
	User          *api.User  `json:"user,omitempty"`
	TokenName     string     `json:"token_name,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type perimeterSummary struct {
	Managed     bool   `json:"managed"`
	ManagedText string `json:"managed_status"`
}

type agentStatusLine struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Managed  bool   `json:"managed"`
	Hook     string `json:"hook"`
	MCP      string `json:"mcp"`
	Enforced string `json:"enforcement"`
}

type meWithTokenName struct {
	apicontract.MeResponse
	TokenName string `json:"token_name"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("status: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("status: resolve repo root: %w", err)
	}

	policyDBPath := filepath.Join(absRoot, ".cordon", "policy.db")
	initialised := store.HasPerimeterID(policyDBPath)

	out := statusOutput{
		Repository: repositoryStatus{
			Root:        absRoot,
			Initialised: initialised,
		},
		Policy:    policyStatus{},
		Auth:      authSummary{Authenticated: false},
		Perimeter: perimeterSummary{Managed: false, ManagedText: "unknown"},
	}

	var installedAgentIDs []string
	if initialised {
		out.Repository.PolicyDB = policyDBPath
		if err := loadLocalPolicyStatus(absRoot, &out, &installedAgentIDs); err != nil {
			return fmt.Errorf("status: %w", err)
		}
	}

	authInfo, authErr := resolveAuthSummary()
	if authErr != nil {
		return fmt.Errorf("status: auth: %w", authErr)
	}
	out.Auth = authInfo

	if initialised && out.Repository.PerimeterID != "" && out.Auth.Authenticated {
		out.Perimeter = resolveManagedPerimeter(out.Repository.PerimeterID)
	}

	out.Agents = collectAgentStatus(absRoot, installedAgentIDs)

	if flags.JSON {
		payload, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(payload))
		return nil
	}
	renderStatusText(cmd, out)
	return nil
}

func loadLocalPolicyStatus(absRoot string, out *statusOutput, installedAgentIDs *[]string) error {
	pdb, err := sql.Open("sqlite", filepath.Join(absRoot, ".cordon", "policy.db"))
	if err != nil {
		return fmt.Errorf("open policy database: %w", err)
	}
	defer pdb.Close()

	if _, err := pdb.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		return fmt.Errorf("set policy busy timeout: %w", err)
	}

	if err := store.MigratePolicyDB(pdb); err != nil {
		return fmt.Errorf("migrate policy database: %w", err)
	}

	perimeterID, err := store.GetPerimeterID(pdb)
	if err == nil {
		out.Repository.PerimeterID = perimeterID
		dataDBPath, dataErr := store.DataDBPathFromID(perimeterID)
		if dataErr == nil {
			out.Repository.DataDB = dataDBPath
		}
	}

	fileRules, err := store.ListFileRules(pdb)
	if err != nil {
		return fmt.Errorf("list file rules: %w", err)
	}
	commandRules, err := store.ListRules(pdb)
	if err != nil {
		return fmt.Errorf("list command rules: %w", err)
	}
	out.Policy.FileRules = len(fileRules)
	out.Policy.CommandRules = len(commandRules)

	agentsFromMeta, err := store.GetInstalledAgents(pdb)
	if err == nil && agentsFromMeta != nil {
		*installedAgentIDs = agentsFromMeta
	}

	dataDBPath, err := store.DataDBPath(absRoot)
	if err != nil {
		return nil
	}
	ddb, err := sql.Open("sqlite", dataDBPath)
	if err != nil {
		return nil
	}
	defer ddb.Close()
	if _, err := ddb.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		return nil
	}
	if err := store.MigrateDataDB(ddb); err != nil {
		return nil
	}
	passes, err := store.ListPasses(ddb)
	if err == nil {
		out.Policy.ActivePasses = len(passes)
	}
	return nil
}

func resolveAuthSummary() (authSummary, error) {
	token, tokenType, err := api.ResolveToken()
	if err != nil {
		return authSummary{}, err
	}
	if token == "" {
		return authSummary{Authenticated: false}, nil
	}

	creds, _ := api.LoadCredentials()
	client := api.NewClientWithToken(token)

	var me meWithTokenName
	_, err = client.GetJSON("/api/v1/auth/me", &me)
	if err != nil {
		if errors.Is(err, api.ErrUnauthorized) {
			_ = api.ClearCredentials()
			return authSummary{Authenticated: false}, nil
		}
		// Token is present but currently unverifiable (offline/server error).
		return authSummary{
			Authenticated: true,
			AuthType:      tokenType,
		}, nil
	}

	user := &api.User{
		ID:          me.User.Id,
		Username:    me.User.Username,
		DisplayName: me.User.DisplayName,
	}

	summary := authSummary{
		Authenticated: true,
		AuthType:      tokenType,
		User:          user,
	}
	if tokenType == api.CredentialTypeMachine {
		summary.TokenName = me.TokenName
		if summary.TokenName == "" && creds != nil {
			summary.TokenName = creds.TokenName
		}
	} else if creds != nil {
		summary.ExpiresAt = &creds.ExpiresAt
	}
	return summary, nil
}

func resolveManagedPerimeter(perimeterID string) perimeterSummary {
	client, err := api.NewClient()
	if err != nil {
		return perimeterSummary{Managed: false, ManagedText: "unknown"}
	}
	_, ok, err := policysync.LookupPerimeter(client, perimeterID)
	if err != nil {
		return perimeterSummary{Managed: false, ManagedText: "unknown"}
	}
	if ok {
		return perimeterSummary{Managed: true, ManagedText: "yes"}
	}
	return perimeterSummary{Managed: false, ManagedText: "no"}
}

func collectAgentStatus(repoRoot string, installedAgentIDs []string) []agentStatusLine {
	managed := make(map[string]bool, len(installedAgentIDs))
	for _, id := range installedAgentIDs {
		managed[id] = true
	}

	allAgents := slices.Clone(agents.All())
	slices.SortStableFunc(allAgents, func(a, b agents.Agent) int {
		return agentOrderRank(a.ID()) - agentOrderRank(b.ID())
	})

	lines := make([]agentStatusLine, 0, len(allAgents))
	for _, a := range allAgents {
		hook := "not configured"
		if a.Installed(repoRoot) {
			hook = "configured"
		}

		mcp := "unsupported"
		if a.SupportsMCPElicitation() {
			if hasMCPConfig(repoRoot, a.ID()) {
				mcp = "configured"
			} else {
				mcp = "not configured"
			}
		}

		lines = append(lines, agentStatusLine{
			ID:       a.ID(),
			Name:     a.DisplayName(),
			Managed:  managed[a.ID()],
			Hook:     hook,
			MCP:      mcp,
			Enforced: enforcementLevel(a.SupportsMCPElicitation()),
		})
	}
	return lines
}

func agentOrderRank(agentID string) int {
	switch agentID {
	case "codex":
		return 0
	case "claude-code":
		return 1
	case "cursor":
		return 2
	case "vs-copilot":
		return 3
	case "gemini-cli":
		return 4
	case "opencode":
		return 5
	default:
		return 99
	}
}

func enforcementLevel(mcpSupported bool) string {
	if !mcpSupported {
		return "hard hook, no MCP"
	}
	return "hard hook + MCP"
}

func hasMCPConfig(repoRoot, agentID string) bool {
	switch agentID {
	case "claude-code":
		return hasMCPServer(filepath.Join(repoRoot, config.MCPRelPath), "mcpServers", config.CordonMCPKey)
	case "cursor":
		return hasMCPServer(filepath.Join(repoRoot, config.CursorMCPRelPath), "mcpServers", config.CordonMCPKey)
	case "vs-copilot":
		return hasMCPServer(filepath.Join(repoRoot, config.VSCodeMCPRelPath), "servers", config.CordonMCPKey)
	case "codex":
		return codexHasMCP(filepath.Join(repoRoot, config.CodexConfigRelPath))
	case "opencode":
		return openCodeHasMCP(filepath.Join(repoRoot, ".opencode", "opencode.jsonc"))
	default:
		return false
	}
}

func hasMCPServer(path, topLevelKey, serverKey string) bool {
	data, err := config.ReadSettings(path)
	if err != nil {
		return false
	}
	raw, ok := data[topLevelKey]
	if !ok {
		return false
	}
	servers, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = servers[serverKey]
	return ok
}

func codexHasMCP(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	txt := string(content)
	return strings.Contains(txt, "[mcp_servers.cordon]") &&
		strings.Contains(txt, "command = \"cordon\"")
}

func openCodeHasMCP(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	clean := stripJSONC(raw)
	var data map[string]interface{}
	if err := json.Unmarshal(clean, &data); err != nil {
		return false
	}
	mcpRaw, ok := data["mcp"]
	if !ok {
		return false
	}
	mcp, ok := mcpRaw.(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = mcp["cordon"]
	return ok
}

// stripJSONC removes comments and trailing commas from JSONC.
func stripJSONC(raw []byte) []byte {
	return stripJSONCTrailingCommas(stripJSONCComments(raw))
}

func stripJSONCComments(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escape := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				out = append(out, ch)
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(raw) && raw[i+1] == '/' {
				inBlockComment = false
				i++
				continue
			}
			if ch == '\n' {
				out = append(out, ch)
			}
			continue
		}
		if inString {
			out = append(out, ch)
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == '/' && i+1 < len(raw) {
			next := raw[i+1]
			if next == '/' {
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}
		out = append(out, ch)
	}
	return out
}

func stripJSONCTrailingCommas(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escape := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			out = append(out, ch)
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(raw) {
				switch raw[j] {
				case ' ', '\t', '\n', '\r':
					j++
				default:
					goto nextToken
				}
			}
		nextToken:
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}
		}
		out = append(out, ch)
	}
	return out
}

func renderStatusText(cmd *cobra.Command, out statusOutput) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Repository: %s\n", out.Repository.Root)
	if !out.Repository.Initialised {
		fmt.Fprintln(w, "Initialised: no (run \"cordon init\")")
	} else {
		fmt.Fprintf(w, "Initialised: yes\n")
		if out.Repository.PerimeterID != "" {
			fmt.Fprintf(w, "Perimeter ID: %s\n", out.Repository.PerimeterID)
		}
	}

	if out.Auth.Authenticated {
		name := "unknown"
		if out.Auth.User != nil && out.Auth.User.Username != "" {
			name = out.Auth.User.Username
			if out.Auth.User.DisplayName != "" && out.Auth.User.DisplayName != out.Auth.User.Username {
				name = name + " (" + out.Auth.User.DisplayName + ")"
			}
		}
		if out.Auth.AuthType == api.CredentialTypeMachine && out.Auth.TokenName != "" {
			fmt.Fprintf(w, "Auth: logged in as %s via machine token (%s)\n", name, out.Auth.TokenName)
		} else {
			fmt.Fprintf(w, "Auth: logged in as %s via %s\n", name, authTypeLabel(out.Auth.AuthType))
		}
	} else {
		fmt.Fprintln(w, "Auth: not authenticated")
	}
	fmt.Fprintf(w, "Managed Perimeter: %s\n", out.Perimeter.ManagedText)

	if out.Repository.Initialised {
		fmt.Fprintf(w, "Policy: %d file rules, %d command rules, %d active passes\n",
			out.Policy.FileRules, out.Policy.CommandRules, out.Policy.ActivePasses)
	}

	fmt.Fprintln(w, "\nAgents:")
	for _, a := range out.Agents {
		managedLabel := "unmanaged"
		if a.Managed {
			managedLabel = "managed"
		}
		fmt.Fprintf(w, "  %-13s  %-9s  hook=%-14s  mcp=%-13s\n", a.Name, managedLabel, a.Hook, a.MCP)
	}
}

func authTypeLabel(t string) string {
	switch t {
	case api.CredentialTypeMachine:
		return "machine token"
	case api.CredentialTypeOAuth:
		return "OAuth"
	default:
		return "token"
	}
}
