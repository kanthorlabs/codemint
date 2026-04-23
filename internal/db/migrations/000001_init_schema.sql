-- +goose Up
-- 1. PROJECT (Core context)
CREATE TABLE IF NOT EXISTS project (
    id TEXT PRIMARY KEY, -- UUIDv7
    name TEXT NOT NULL UNIQUE,
    working_dir TEXT NOT NULL,
    yolo_mode INTEGER NOT NULL DEFAULT 0 -- EPIC-04: Persists the /yolo toggle (0=Off, 1=On)
);

-- 2. PROJECT PERMISSION (EPIC-03: The Auto-Approval Interceptor)
CREATE TABLE IF NOT EXISTS project_permission (
    id TEXT PRIMARY KEY, -- UUIDv7
    project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    allowed_commands TEXT, -- JSON array of whitelisted commands (e.g., '["go test"]')
    allowed_directories TEXT, -- JSON array of safe paths
    blocked_commands TEXT -- JSON array of strictly forbidden commands (e.g., '["rm"]')
);

-- 3. AGENT (The Actors)
CREATE TABLE IF NOT EXISTS agent (
    id TEXT PRIMARY KEY, -- UUIDv7
    name TEXT NOT NULL UNIQUE, -- e.g., 'human', 'sys-auto-approve', 'opencode'
    type INTEGER NOT NULL, -- 0: Human, 1: Assistant, 2: System
    assistant TEXT -- Maps to config.yaml keys for API routing
);

-- 4. SESSION (The Instance)
CREATE TABLE IF NOT EXISTS session (
    id TEXT PRIMARY KEY, -- UUIDv7
    project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    status INTEGER NOT NULL DEFAULT 0 -- 0: Active, 1: Archived
);

-- EPIC-01: CRITICAL Constraint - Only one active session per project
CREATE UNIQUE INDEX IF NOT EXISTS idx_active_session ON session (project_id) WHERE status = 0;

-- 5. WORKFLOW (The Orchestration Group)
CREATE TABLE IF NOT EXISTS workflow (
    id TEXT PRIMARY KEY, -- UUIDv7
    session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    type INTEGER NOT NULL -- Enum mapped in Go registry (e.g., Project Coding, Checking)
);

-- 6. TASK (The Atomic Unit of Work)
CREATE TABLE IF NOT EXISTS task (
    id TEXT PRIMARY KEY, -- UUIDv7
    project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    workflow_id TEXT REFERENCES workflow(id) ON DELETE CASCADE, -- Nullable if standalone
    assignee_id TEXT NOT NULL REFERENCES agent(id),
    
    -- EPIC-02: Hierarchical Ordering (The linear execution graph)
    seq_epic INTEGER NOT NULL DEFAULT 0,
    seq_story INTEGER NOT NULL DEFAULT 0,
    seq_task INTEGER NOT NULL DEFAULT 0,
    
    -- Enums & State
    type INTEGER NOT NULL, -- 0:Coding, 1:Verification, 2:Confirmation, 3:Coordination, 4:Retrospective
    status INTEGER NOT NULL DEFAULT 0, -- 0:pending, 1:processing, 2:awaiting, 3:success, 4:failure, 5:completed, 6:reverted, 7:cancelled
    
    -- Payloads
    input TEXT, -- JSON payload of context/prompt sent to Agent
    output TEXT -- JSON payload of the result/diff/feedback received from Agent or Human
);

-- +goose Down
DROP TABLE IF EXISTS task;
DROP TABLE IF EXISTS workflow;
DROP INDEX IF EXISTS idx_active_session;
DROP TABLE IF EXISTS session;
DROP TABLE IF EXISTS agent;
DROP TABLE IF EXISTS project_permission;
DROP TABLE IF EXISTS project;