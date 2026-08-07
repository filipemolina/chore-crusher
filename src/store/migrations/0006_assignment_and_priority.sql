-- 0006_assignment_and_priority.sql — durable task assignment and task priority.
--
-- assignee is NOT presence. AgentActivity (0002) is a 120-second spinner
-- heartbeat; this column has no TTL and changes only when someone explicitly
-- assigns, unassigns, or completes the task. See
-- docs/plan/mcp-assignment-and-priorities.md §3 and docs/DESIGN.md §3.
ALTER TABLE Task ADD COLUMN assignee    text    not null default '';
ALTER TABLE Task ADD COLUMN assigned_at integer;
ALTER TABLE Task ADD COLUMN priority    text    not null default 'none';

CREATE INDEX IF NOT EXISTS idx_task_assignee ON Task(assignee);
