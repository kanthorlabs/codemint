# Retro: Story 2.0 — Planning Lessons

What the plan got right vs. what bit us mid-implementation. Each item is a concrete planning gap, why it cost us, and what to put on the checklist next time.

---

## 1. The plan trusted the existing ACP transport. The transport was wrong.

**What happened.** Tasks 2.0.5 and 2.0.7 assumed that once routing was in place, freeform input would reach the assistant. It didn't. The `initialize`, `session/new`, and `session/prompt` payloads we were sending were not spec-compliant:

- `InitializeParams` was missing the required `protocolVersion` field.
- `SessionNewParams` was missing required `cwd` and `mcpServers` fields (and `mcpServers` must serialize as `[]`, not `null`).
- `SessionPromptParams.Prompt` was a `string`; the spec requires `[]ContentBlock`.

OpenCode rejected the handshake silently or never produced events, and we spent debugging time on the dispatcher and runtime before finding the bug at the wire.

**Why the plan missed it.** The plan listed *what to wire up*, not *what to verify works first*. ACP compliance was treated as a precondition that already held. It didn't.

**Lesson — checklist for next plan.** Before any user-facing story that depends on a transport, include a Task 0: "Run a one-prompt round-trip against the chosen ACP child and capture the wire log. Confirm `initialize` → `session/new` → `session/prompt` → `session/update` succeeds with the *current* binary, not last quarter's." This is 30 minutes of work that would have saved hours.

## 2. "Replace `IsGlobal` with kind-based predicate" hid an interface change.

**What happened.** The plan named two surface changes (`ActiveSession.IsGlobal` → `IsCodeMintSession()`, `registry.ActiveSessionInfo.GetIsGlobal` → `GetIsCodeMint`). It did not list a *third*: `registry.MutableSessionInfo` needed a new `GetProjectID()` method, because the new `/project-assistant` handler reaches into the active session through that interface — but the interface only exposed `GetSessionID()`.

**Why the plan missed it.** Task 2.0.8 specified the handler behavior but not the shape of the data flow from the registry boundary to the project repo. The plan treated `ActiveSession` as if handlers had direct access; in reality, handlers see `registry.ActiveSessionInfo` / `registry.MutableSessionInfo`.

**Lesson — checklist for next plan.** When a new command is added, walk the *call graph from the handler signature outward* and list every interface method it transitively touches. Any method not on the interface today is a planned change, not an "obvious extension."

## 3. `AttachWorker` couldn't be reused for chat — and the plan didn't notice.

**What happened.** The bootstrap put the user into a CodeMint session. Freeform input routed to `SystemAssistant.Ask`, which called `Runtime.AttachWorker` to get a worker, then read `worker.Out()` directly. But `AttachWorker` *also* spawns a `Pipeline + StatusMapper + PipelineConsumer` chain that consumes the same `worker.Out()`. Two consumers, one channel — events were stolen by the Pipeline and the assistant saw nothing.

We added `AttachWorkerRaw` (worker only, no pipeline) to fix it. That's a new public method on the `WorkerAttacher` interface and a test-mock update in `agent/system_assistant_test.go`.

**Why the plan missed it.** The plan inherited the existing `WorkerAttacher` from Story 3.21/3.22 and assumed it was generic. It wasn't — it was task-oriented. The chat path has different needs: no task, no status mapping, just stream events to the user.

**Lesson — checklist for next plan.** When a story makes an existing component serve a new caller, include a "second-consumer audit" task: list every channel/goroutine the component owns and confirm exclusive ownership semantics for the new caller. "Reuse `X`" is a planning hypothesis, not a free shortcut.

## 4. The plan deferred decisions that turned into mid-implementation cuts.

**What happened.** Two unplanned cuts shipped in this story:

- `internal/agent/provider_catalog.go` lost the `codex` and `claude-code` builtins. Story 2.0 was supposed to be neutral on providers; in practice, those entries pointed at binaries with broken or untested ACP behavior, so they were producing "session opens but nothing happens" failures during manual testing. Easier to delete than diagnose.
- `configs/config.yaml.example` had its `workflows:` block commented out. The plan assumed workflow routing was usable; the dispatcher's Coding-kind branch falls through to `ErrNoBrainstormer` anyway, so workflows were noise that misled users about what's supported.

**Why the plan missed it.** Story 2.0 listed acceptance criteria for *features* but not *user-visible defaults*. A fresh-install user runs `codemint`, they get whatever the example config and built-in catalog say. The plan never asked "what does fresh-install actually do?"

**Lesson — checklist for next plan.** Add a "fresh-install dry-run" verification step to every story that touches bootstrap: `rm -rf $XDG_DATA_HOME/codemint && cp configs/config.yaml.example $XDG_CONFIG_HOME/codemint/config.yaml && ./build/codemint`. Anything broken or noisy in that flow blocks the story.

## 5. Idempotency was specified but not the *mechanism*.

**What happened.** Task 2.0.2 said "Ensure a `project_permission` row exists for the project. If absent: `domain.NewProjectPermission(project.ID)` → `permRepo.Create`." The actual implementation uses `permRepo.Upsert` instead, because the repo already exposed an upsert that does the same job in one call without a read-then-write race.

That's fine — the implementer made a better call. But a re-read of the plan would not have caught this drift, because it specifies the strategy too tightly. If the implementer had blindly followed `Create`, they'd have shipped an idempotency bug.

**Lesson — checklist for next plan.** Specify idempotency *contracts* (post-state invariants), not *call sequences*. "After this function returns nil, exactly one permission row exists with `project_id = p.ID`" is implementable many ways. "Find then Create" is one specific way that may be wrong on race or duplicate-key.

## 6. The boot sequence was constrained but the constraint wasn't called out.

**What happened.** `cmd/codemint/main.go` constructed `permissionRepo` *after* the project bootstrap was supposed to run. Implementing Task 2.0.3 required moving `sqlite.NewProjectPermissionRepo(dbConn)` earlier so `EnsureCodeMintProject` could take it as a dependency.

That's a one-line move, but the plan said "immediately after `agentRepo.EnsureSystemAgents`" without flagging that `permissionRepo` wasn't constructed yet at that point.

**Why the plan missed it.** The plan referenced the boot sequence by step number (Step 6, Step 7…) without re-reading what each step *currently* did. It treated the `run()` function as a known shape rather than re-verifying it.

**Lesson — checklist for next plan.** When inserting into a long sequence (boot order, middleware chain, migration order), paste the *current* sequence into the plan and mark the insertion point inline. This forces the planner to look at neighbors and catch dependency moves.

---

## Meta-lesson

All six items have the same shape: **the plan specified the change, but did not specify the verification.** "Add a column" is easy to get right. "Add a column *and prove the migration runs cleanly on a fresh DB*" catches half the bugs. Good plans include a verification clause that an implementer can mechanically check, not a goal an implementer can hand-wave past.

Next plan template should make verification a *required field* per task, and the verification should be executable (a command to run, not a property to claim).
