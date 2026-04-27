# User Story 2.0.4: Workflow Command

* **As a** developer,
* **I want to** start a workflow via `/workflow <name>`,
* **So that** I can begin a structured brainstorming or coding session.

## Acceptance Criteria

1. `/workflow` with no args lists available workflows.
2. `/workflow <name>` starts the named workflow.
3. Tab autocomplete suggests workflow names.
4. Workflow tasks are generated and inserted into SQLite with correct seq_epic/seq_story/seq_task.
5. Guardrails (Verification, Confirmation, Retrospective) are auto-injected unless opted out.

## Technical Design

### Command Interface

```
> /workflow
Available workflows:
  brainstorming    5-phase brainstorming pipeline for feature planning

> /workflow brainstorming
Starting workflow: brainstorming (v1.0)

Phase 1: Context Intake
Reading project structure...
```

### Task Generation Flow

```
1. User runs /workflow brainstorming
2. Command looks up "brainstorming" in FileRegistry
3. TaskGenerator.GenerateTasks(wf, sessionID) creates Task records:
   - Parse each Epic → Story
   - Resolve skill references
   - Set seq_epic, seq_story, seq_task
   - Auto-inject guardrails based on settings
   - Set depends_on/condition for routes
4. Insert tasks into SQLite
5. Create Workflow execution record (file_path, started_at, status=Active)
6. Scheduler picks up first pending task
```

### Guardrail Auto-Injection

```go
func (g *TaskGenerator) GenerateTasks(wf *domain.WorkflowFile, sessionID string) ([]*domain.Task, error) {
    var tasks []*domain.Task
    
    for epicIdx, epic := range wf.Epics {
        for storyIdx, story := range epic.Stories {
            // Create main story task
            task := g.createStoryTask(epic, story, epicIdx, storyIdx)
            tasks = append(tasks, task)
            
            // Resolve guardrail settings
            guardrails := story.Guardrails
            if guardrails == nil {
                guardrails = &wf.Settings.Guardrails
            }
            
            // Auto-inject verification after coding tasks
            if guardrails.Verification && story.Type == domain.TaskTypeCoding {
                tasks = append(tasks, g.createVerificationTask(task))
            }
            
            // Auto-inject confirmation at end of story
            if guardrails.Confirmation {
                tasks = append(tasks, g.createConfirmationTask(task))
            }
        }
        
        // Auto-inject retrospective at end of epic
        retrospective := epic.Retrospective
        if retrospective == nil {
            retrospective = &wf.Settings.Guardrails.Retrospective
        }
        if *retrospective {
            tasks = append(tasks, g.createRetrospectiveTask(epic, epicIdx))
        }
    }
    
    return tasks, nil
}
```

## Dependencies

- 2.0.1 Workflow File Infrastructure (FileRegistry, parser)
- 2.0.2 Task Routing (depends_on, condition)
- 2.0.3 Execution State (workflow record creation)

## Blocks

- 2.1 Phase 1 - Context Intake
- All subsequent workflow phases
