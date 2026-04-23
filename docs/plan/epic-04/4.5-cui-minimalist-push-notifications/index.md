# User Story 4.5: CUI Minimalist Push Notifications

* **As a** Developer using Telegram/Slack,
* **I want** the bot to remain silent unless it hits a blocking state or a major milestone,
* **So that** my phone is not spammed with notification fatigue.
* *Acceptance Criteria:*
    * The CUI does *not* auto-update a pinned context message.
    * Push notifications are only triggered for `awaiting` human input tasks or milestone completions based on the Verbosity Filter.
