package buildinfo

// Version is set at build time via:
// -ldflags "-X github.com/cordon-co/cordon-cli/cli/internal/buildinfo.Version=<tag>"
var Version = "dev"
