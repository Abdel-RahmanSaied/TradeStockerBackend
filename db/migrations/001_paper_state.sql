-- Local paper prototype only. One locked aggregate makes mutations serializable.
CREATE TABLE IF NOT EXISTS schema_migrations (
  version integer PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS paper_workspace (
  id integer PRIMARY KEY CHECK (id = 1),
  mode text NOT NULL DEFAULT 'PAPER' CHECK (mode = 'PAPER'),
  document jsonb NOT NULL CHECK (jsonb_typeof(document) = 'object'),
  updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations(version) VALUES (1) ON CONFLICT DO NOTHING;
