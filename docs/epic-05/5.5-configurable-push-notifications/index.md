# User Story 5.5: Configurable Push Notifications

* **As the** Go Orchestrator,
* **I want to** read a cron-style schedule from `config.yaml` (`memory.review_alert`),
* **So that** I know exactly when to trigger the `UIAdapter` to notify the user about pending insights.
* *Acceptance Criteria:*
    * A background Go routine parses the cron string.
    * When triggered, if `index.md` is not empty, it sends a non-intrusive alert payload to the UIMediator.
