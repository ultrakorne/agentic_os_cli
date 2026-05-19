// Package build holds values injected by the linker at build time.
//
// Goreleaser sets Version/Commit/Date via `-ldflags "-X"` for tagged
// release builds (see .goreleaser.yaml). Local builds keep the zero
// values, which `aos version` renders as "dev".
package build

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)
