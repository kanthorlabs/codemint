package workflow

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/skills"
)

// FileRegistry discovers and loads WORKFLOW.yaml files from multiple sources.
// It provides access to workflow definitions for the workflow command and routing.
//
// Load order (ascending precedence):
// 1. External: ~/.local/share/codemint/workflows/
// 2. Embedded: internal/workflow/embedded/ (highest precedence)
//
// When the same workflow name exists in multiple sources, the higher-precedence
// source wins.
type FileRegistry struct {
	workflows map[string]*domain.WorkflowFile
	parser    *WorkflowParser
}

// NewFileRegistry creates a new empty FileRegistry. Call LoadAll to populate it.
func NewFileRegistry() *FileRegistry {
	return &FileRegistry{
		workflows: make(map[string]*domain.WorkflowFile),
		parser:    NewWorkflowParser(),
	}
}

// LoadAll discovers and loads workflows from all configured sources.
// External directories that do not exist are silently skipped.
// Returns an error if any WORKFLOW.yaml file fails to parse.
//
// If skillResolver is non-nil, all Story.Skill references are validated
// against the registry. Workflows with unresolvable skill references are
// rejected with an error (L1 validation per AC §2.0.5).
func (r *FileRegistry) LoadAll(skillResolver skills.SkillResolver) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("workflow: resolve home dir: %w", err)
	}

	// Load external workflows first (lower precedence)
	externalDir := filepath.Join(home, ".local", "share", "codemint", "workflows")
	if err := r.loadDir(externalDir); err != nil {
		return err
	}

	// Load embedded workflows last (highest precedence)
	if err := r.loadEmbedded(); err != nil {
		return fmt.Errorf("workflow: load embedded: %w", err)
	}

	// Validate all skill references if resolver is provided.
	if skillResolver != nil {
		if err := r.validateSkillReferences(skillResolver); err != nil {
			return err
		}
	}

	return nil
}

// validateSkillReferences checks that all Story.Skill references resolve
// against the skills registry. Returns an error for the first unresolvable
// reference, rejecting the entire workflow.
func (r *FileRegistry) validateSkillReferences(resolver skills.SkillResolver) error {
	for _, wf := range r.workflows {
		for epicIdx, epic := range wf.Epics {
			for storyIdx, story := range epic.Stories {
				if story.Skill == "" {
					continue // skill is optional
				}
				if _, ok := resolver.Get(story.Skill); !ok {
					return fmt.Errorf(
						"workflow %q references unknown skill %q at epic[%d].story[%d] %q",
						wf.Name, story.Skill, epicIdx, storyIdx, story.ID,
					)
				}
			}
		}
	}
	return nil
}

// Get returns the workflow with the given name, if present.
func (r *FileRegistry) Get(name string) (*domain.WorkflowFile, bool) {
	wf, ok := r.workflows[name]
	return wf, ok
}

// All returns all registered workflows sorted by name.
func (r *FileRegistry) All() []*domain.WorkflowFile {
	wfs := make([]*domain.WorkflowFile, 0, len(r.workflows))
	for _, wf := range r.workflows {
		wfs = append(wfs, wf)
	}
	slices.SortFunc(wfs, func(a, b *domain.WorkflowFile) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return wfs
}

// Names returns all registered workflow names sorted alphabetically.
// Useful for autocomplete.
func (r *FileRegistry) Names() []string {
	names := make([]string, 0, len(r.workflows))
	for name := range r.workflows {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Len returns the number of registered workflows.
func (r *FileRegistry) Len() int {
	return len(r.workflows)
}

// loadDir scans the given directory for workflow subdirectories and loads each one.
// Non-existent directories are silently skipped.
func (r *FileRegistry) loadDir(dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workflow: read dir %q: %w", dirPath, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workflowDir := filepath.Join(dirPath, entry.Name())
		// Skip directories that do not contain a WORKFLOW.yaml file.
		if _, err := os.Stat(filepath.Join(workflowDir, "WORKFLOW.yaml")); os.IsNotExist(err) {
			continue
		}
		wf, err := r.parser.Parse(workflowDir)
		if err != nil {
			return fmt.Errorf("workflow: parse %q: %w", workflowDir, err)
		}
		r.workflows[wf.Name] = wf
	}
	return nil
}

// loadEmbedded extracts embedded workflows to a temp directory, parses them, and
// registers them with the highest precedence.
func (r *FileRegistry) loadEmbedded() error {
	tmpDir, err := os.MkdirTemp("", "codemint-workflows-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractEmbeddedWorkflows(tmpDir); err != nil {
		return fmt.Errorf("extract embedded workflows: %w", err)
	}

	return r.loadDir(filepath.Join(tmpDir, "embedded"))
}

// extractEmbeddedWorkflows writes the embedded FS contents rooted at "embedded" into dst.
func extractEmbeddedWorkflows(dst string) error {
	return fs.WalkDir(embeddedWorkflowFS, "embedded", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(dst, path)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := embeddedWorkflowFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %q: %w", path, err)
		}
		return os.WriteFile(target, data, 0o644)
	})
}
