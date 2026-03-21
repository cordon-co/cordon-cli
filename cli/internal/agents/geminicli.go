package agents

// GeminiCLI is a placeholder stub for Google Gemini CLI agent integration.
type GeminiCLI struct{}

func (g *GeminiCLI) ID() string                      { return "gemini-cli" }
func (g *GeminiCLI) DisplayName() string              { return "Gemini CLI" }
func (g *GeminiCLI) Installable() bool                { return false }
func (g *GeminiCLI) Install(repoRoot string) error    { return nil }
func (g *GeminiCLI) Remove(repoRoot string) error     { return nil }
func (g *GeminiCLI) Installed(repoRoot string) bool   { return false }
