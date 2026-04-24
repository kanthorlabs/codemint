package domain

// Skill represents an AgentSkills-compliant skill loaded from a SKILL.md file.
// The ID is an MD5 hash of the absolute path to the SKILL.md file, ensuring
// deterministic and environment-consistent identification.
type Skill struct {
	// ID is the MD5 hash of the absolute path to SKILL.md.
	ID string

	// Name is required. Must match the parent directory name. 1-64 chars, lowercase, hyphens.
	Name string

	// Description is required. 1-1024 chars. What the skill does + when to use it.
	Description string

	// License is optional. License name or reference to a bundled file.
	License string

	// Compatibility is optional. 1-500 chars. Environment requirements.
	Compatibility string

	// Metadata is optional. Arbitrary key-value pairs.
	Metadata map[string]string

	// AllowedTools is optional. Space-separated pre-approved tools (experimental).
	AllowedTools string

	// Instruction is the parsed Markdown body from SKILL.md (excluding frontmatter).
	Instruction string

	// Scripts is the list of executable scripts found in the scripts/ directory.
	Scripts []SkillScript

	// References is the list of file paths found in the references/ directory.
	References []string

	// SourceDir is the absolute path to the skill directory.
	SourceDir string
}

// SkillScript represents a named executable script bundled with a skill.
type SkillScript struct {
	// Name is the script identifier, e.g., "lint".
	Name string

	// Executable is the relative path to the script, e.g., "scripts/lint.sh".
	Executable string
}
