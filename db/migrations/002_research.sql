-- SEC research jobs and their original sources. Kept apart from paper financial state.
CREATE TABLE IF NOT EXISTS research_jobs (
 id text PRIMARY KEY CHECK (id ~ '^research_[0-9a-f]{32}$'),
 operation_key text NOT NULL UNIQUE,
 request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
 symbol text NOT NULL CHECK (symbol ~ '^[A-Z][A-Z0-9.-]{0,15}$'),
 cik text NOT NULL CHECK (cik ~ '^[0-9]{10}$'),
 status text NOT NULL CHECK (status IN ('QUEUED','RUNNING','SUCCEEDED','FAILED')),
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 not_before timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 result jsonb,
 error_code text,
 last_error text,
 CHECK ((status = 'RUNNING') = (lease_until IS NOT NULL)),
 CHECK ((status = 'SUCCEEDED') = (result IS NOT NULL)),
 CHECK ((status = 'FAILED') = (error_code IS NOT NULL))
);
CREATE TABLE IF NOT EXISTS research_sources (
 job_id text NOT NULL REFERENCES research_jobs(id),
 attempt integer NOT NULL CHECK (attempt BETWEEN 1 AND 3),
 kind text NOT NULL CHECK (kind IN ('submissions','companyfacts')),
 sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
 url text NOT NULL CHECK (url LIKE 'https://data.sec.gov/%'),
 received_at timestamptz NOT NULL,
 body bytea NOT NULL,
 PRIMARY KEY (job_id, attempt, kind)
);
CREATE TABLE IF NOT EXISTS research_events (
 sequence bigserial PRIMARY KEY,
 job_id text NOT NULL REFERENCES research_jobs(id),
 at timestamptz NOT NULL DEFAULT now(),
 action text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS research_jobs_queue ON research_jobs (not_before, created_at) WHERE status IN ('QUEUED','RUNNING');
-- Provider-wide cooldown. When SEC limits access, no job contacts it until not_before.
CREATE TABLE IF NOT EXISTS research_provider_state (
 provider text PRIMARY KEY CHECK (provider = 'SEC'),
 not_before timestamptz NOT NULL,
 reason text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);

-- Original sources and the event log are append-only for the application.
-- A database administrator can still bypass this; it is not tamper-proof storage.
CREATE OR REPLACE FUNCTION research_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END $$;
DROP TRIGGER IF EXISTS research_sources_append_only ON research_sources;
CREATE TRIGGER research_sources_append_only BEFORE UPDATE OR DELETE ON research_sources FOR EACH ROW EXECUTE FUNCTION research_append_only();
DROP TRIGGER IF EXISTS research_events_append_only ON research_events;
CREATE TRIGGER research_events_append_only BEFORE UPDATE OR DELETE ON research_events FOR EACH ROW EXECUTE FUNCTION research_append_only();

INSERT INTO schema_migrations(version) VALUES (2) ON CONFLICT DO NOTHING;
