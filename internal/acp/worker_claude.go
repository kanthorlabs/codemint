package acp

// Package acp provides the Agent Communication Protocol implementation.
//
// Core concept of how we can run Claude as a worker and continue existing session with new request.
// claude -p \
//   --output-format stream-json \
//   --model sonnet \
//   --permission-mode bypassPermissions \
//   --verbose \
//   --mcp-config /path/to/mcp-config.json \
//   --resume <session-uuid>  # hoặc --session-id nếu session mới \
//   --disallowedTools Bash,Edit,Read,Write,Glob,Grep,... \
//   --settings /path/to/hooks-settings.json \
//   -- "user message"
