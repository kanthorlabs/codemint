# User Story 5.6: The Neglected Inbox TTL

* **As the** Go Orchestrator,
* **I want to** run a daily routine that checks the `<yyyyMMDD>` timestamp inside the filenames in the `unverified/` directory,
* **So that** insights neglected for >14 days are auto-archived and the UI doesn't become bloated.
* *Acceptance Criteria:*
    * Files older than 14 days are moved to `inbox/insights/archive/`.
    * The Go backend cleanly parses `index.md` and strips out the text blocks corresponding to the archived files.
