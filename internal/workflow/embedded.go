package workflow

import "embed"

// embeddedWorkflowFS holds the embedded workflow directories.
// Embedded workflows have the highest precedence in the file registry.
//
//go:embed embedded
var embeddedWorkflowFS embed.FS
