package agents

// OpenCode is a placeholder stub for OpenCode agent integration.
type OpenCode struct{}

func (o *OpenCode) ID() string                      { return "opencode" }
func (o *OpenCode) DisplayName() string              { return "OpenCode" }
func (o *OpenCode) Installable() bool                { return false }
func (o *OpenCode) Install(repoRoot string) error    { return nil }
func (o *OpenCode) Remove(repoRoot string) error     { return nil }
func (o *OpenCode) Installed(repoRoot string) bool   { return false }
