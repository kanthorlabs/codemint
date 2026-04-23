// Package repl provides input parsing and core command registration for the
// CodeMint command-line REPL.
package repl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/shlex"
)

// ErrEmptyInput is returned when the raw input is blank.
var ErrEmptyInput = errors.New("repl: empty input")

// ParseInput applies the dual-path parsing strategy:
//
//  1. If raw starts with '/' it is treated as a slash command. The command
//     word (everything between '/' and the first space) is returned in cmd,
//     the remaining text is shell-split into args, and the untouched remainder
//     is returned as rawArgs.
//
//  2. Otherwise isSlash is false, cmd is empty, args is nil, and rawArgs
//     contains the trimmed natural-language input for the System Assistant.
//
// shlex.Split is used for shell-accurate tokenisation (quoted strings,
// backslash escapes). A shlex error is surfaced as a non-nil err.
func ParseInput(raw string) (isSlash bool, cmd string, args []string, rawArgs string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, "", nil, "", ErrEmptyInput
	}

	if !strings.HasPrefix(trimmed, "/") {
		// Natural language path.
		return false, "", nil, trimmed, nil
	}

	// Slash command path.
	// Strip the leading '/' before splitting.
	body := trimmed[1:]

	// Isolate the command word from the rest.
	parts := strings.SplitN(body, " ", 2)
	cmd = parts[0]
	if cmd == "" {
		return false, "", nil, "", fmt.Errorf("repl: bare '/' is not a valid command")
	}

	if len(parts) == 2 {
		rawArgs = strings.TrimSpace(parts[1])
	}

	if rawArgs != "" {
		args, err = shlex.Split(rawArgs)
		if err != nil {
			return false, "", nil, "", fmt.Errorf("repl: tokenise args for %q: %w", cmd, err)
		}
	}

	return true, cmd, args, rawArgs, nil
}
