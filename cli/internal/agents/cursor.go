package agents

// Cursor is a placeholder stub for Cursor IDE agent integration.
type Cursor struct{}

func (c *Cursor) ID() string                      { return "cursor" }
func (c *Cursor) DisplayName() string              { return "Cursor" }
func (c *Cursor) Installable() bool                { return false }
func (c *Cursor) Install(repoRoot string) error    { return nil }
func (c *Cursor) Remove(repoRoot string) error     { return nil }
func (c *Cursor) Installed(repoRoot string) bool   { return false }
