package agents

// KiloCode is a placeholder stub for KiloCode agent integration.
type KiloCode struct{}

func (k *KiloCode) ID() string                     { return "kilocode" }
func (k *KiloCode) DisplayName() string            { return "KiloCode" }
func (k *KiloCode) Installable() bool              { return false }
func (k *KiloCode) SupportsMCPElicitation() bool   { return false }
func (k *KiloCode) Install(repoRoot string) error  { return nil }
func (k *KiloCode) Remove(repoRoot string) error   { return nil }
func (k *KiloCode) Installed(repoRoot string) bool { return false }
