# User Story 5.4: O(1) Inbox Staging

* **As** The Archivist,
* **I want to** write new insights as standalone `<name>-<yyyyMMDD>.md` files and instantly prepend their full text to `inbox/insights/index.md`,
* **So that** the UI can render the `/review` command in O(1) time without traversing directories.
* *Acceptance Criteria:*
    * The state tracker file is created in `unverified/`.
    * The exact same Markdown content is prepended to the top of `index.md`.
