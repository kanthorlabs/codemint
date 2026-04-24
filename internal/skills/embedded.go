package skills

import "embed"

// embeddedFS holds the embedded seniorgodev skill directory.
// Embedded skills have the highest precedence in the registry.
//
//go:embed embedded
var embeddedFS embed.FS
