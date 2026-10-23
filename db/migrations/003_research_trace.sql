-- S01-T05: the W3C trace ID of the request that created a research job, so the
-- worker continues the same trace. Additive and nullable; older jobs have none.
ALTER TABLE research_jobs ADD COLUMN IF NOT EXISTS trace_id text
  CHECK (trace_id IS NULL OR trace_id ~ '^[0-9a-f]{32}$');
-- The retry budget counts recent attempt starts from the event log at each claim.
CREATE INDEX IF NOT EXISTS research_events_attempt_started ON research_events (at) WHERE action = 'ATTEMPT_STARTED';

INSERT INTO schema_migrations(version) VALUES (3) ON CONFLICT DO NOTHING;
