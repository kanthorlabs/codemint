package orchestrator

import (
	"sync"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

// TestActiveSession_ConcurrentAccess verifies that concurrent reads and writes
// to ActiveSession fields do not cause data races. This test exercises the
// mutex-protected access patterns used by Heartbeat (reads) and SetSession (writes).
func TestActiveSession_ConcurrentAccess(t *testing.T) {
	session1 := &domain.Session{ID: "sess-1"}
	session2 := &domain.Session{ID: "sess-2"}
	project1 := &domain.Project{ID: "proj-1", Kind: domain.ProjectKindCodeMint}
	project2 := &domain.Project{ID: "proj-2", Kind: domain.ProjectKindCoding}

	active := NewActiveSession(session1, project1)

	// Run concurrent reads and writes.
	var wg sync.WaitGroup
	const iterations = 1000

	// Writer goroutine: alternates between two sessions.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				active.SetSession(session1, project1, false)
			} else {
				active.SetSession(session2, project2, true)
			}
		}
	}()

	// Reader goroutine 1: reads session.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = active.GetSession()
		}
	}()

	// Reader goroutine 2: reads project.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = active.GetProject()
		}
	}()

	// Reader goroutine 3: reads IsCodeMintSession.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = active.IsCodeMintSession()
		}
	}()

	// Reader goroutine 4: reads suspended state.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = active.GetSuspended()
		}
	}()

	// Reader goroutine 5: reads YOLO state.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = active.GetYoloEnabled()
		}
	}()

	// Writer goroutine 2: toggles suspended state.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			active.SetSuspended(i%2 == 0)
		}
	}()

	wg.Wait()
}

// TestActiveSession_GettersReturnCorrectValues verifies that getters return
// the values set by SetSession and constructors.
func TestActiveSession_GettersReturnCorrectValues(t *testing.T) {
	session := &domain.Session{ID: "test-sess"}
	project := &domain.Project{ID: "test-proj", Kind: domain.ProjectKindCoding}

	// Test NewActiveSession constructor.
	active := NewActiveSession(session, project)
	if got := active.GetSession(); got != session {
		t.Errorf("GetSession() = %v, want %v", got, session)
	}
	if got := active.GetProject(); got != project {
		t.Errorf("GetProject() = %v, want %v", got, project)
	}
	if got := active.GetYoloEnabled(); got != false {
		t.Errorf("GetYoloEnabled() = %v, want false", got)
	}

	// Test NewActiveSessionWithYolo constructor.
	activeYolo := NewActiveSessionWithYolo(session, project, true)
	if got := activeYolo.GetYoloEnabled(); got != true {
		t.Errorf("GetYoloEnabled() = %v, want true", got)
	}

	// Test SetSession updates values.
	newSession := &domain.Session{ID: "new-sess"}
	newProject := &domain.Project{ID: "new-proj", Kind: domain.ProjectKindCodeMint}
	active.SetSession(newSession, newProject, true)
	if got := active.GetSession(); got != newSession {
		t.Errorf("GetSession() after SetSession = %v, want %v", got, newSession)
	}
	if got := active.GetProject(); got != newProject {
		t.Errorf("GetProject() after SetSession = %v, want %v", got, newProject)
	}
	if got := active.GetYoloEnabled(); got != true {
		t.Errorf("GetYoloEnabled() after SetSession = %v, want true", got)
	}

	// Test SetSession with nil clears values.
	active.SetSession(nil, nil, false)
	if got := active.GetSession(); got != nil {
		t.Errorf("GetSession() after SetSession(nil) = %v, want nil", got)
	}
	if got := active.GetProject(); got != nil {
		t.Errorf("GetProject() after SetSession(nil) = %v, want nil", got)
	}
}

// TestActiveSession_SuspendedState verifies SetSuspended and GetSuspended.
func TestActiveSession_SuspendedState(t *testing.T) {
	active := NewActiveSession(nil, nil)

	if got := active.GetSuspended(); got != false {
		t.Errorf("GetSuspended() initial = %v, want false", got)
	}

	active.SetSuspended(true)
	if got := active.GetSuspended(); got != true {
		t.Errorf("GetSuspended() after SetSuspended(true) = %v, want true", got)
	}

	active.SetSuspended(false)
	if got := active.GetSuspended(); got != false {
		t.Errorf("GetSuspended() after SetSuspended(false) = %v, want false", got)
	}
}

// TestActiveSession_IsCodeMintSession verifies IsCodeMintSession returns
// correct values for different project kinds.
func TestActiveSession_IsCodeMintSession(t *testing.T) {
	tests := []struct {
		name    string
		project *domain.Project
		want    bool
	}{
		{
			name:    "nil project",
			project: nil,
			want:    false,
		},
		{
			name:    "CodeMint project",
			project: &domain.Project{Kind: domain.ProjectKindCodeMint},
			want:    true,
		},
		{
			name:    "Coding project",
			project: &domain.Project{Kind: domain.ProjectKindCoding},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := NewActiveSession(nil, tt.project)
			if got := active.IsCodeMintSession(); got != tt.want {
				t.Errorf("IsCodeMintSession() = %v, want %v", got, tt.want)
			}
		})
	}
}
