-- V2 workflow and trace backbone. Demo data is disposable, so new flows write
-- explicit workflow/job/agent/retrieval/artifact traces instead of relying on
-- task status alone.

CREATE TABLE IF NOT EXISTS workflow_runs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_type VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id INTEGER NOT NULL,
    job_id INTEGER REFERENCES background_jobs(id) ON DELETE SET NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    last_error TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT workflow_runs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'))
);

CREATE INDEX IF NOT EXISTS workflow_runs_tenant_resource_idx
    ON workflow_runs (tenant_id, resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS workflow_runs_job_id_idx
    ON workflow_runs (job_id);
CREATE INDEX IF NOT EXISTS workflow_runs_status_idx
    ON workflow_runs (tenant_id, status, created_at DESC);

ALTER TABLE background_jobs
    ADD COLUMN IF NOT EXISTS workflow_run_id INTEGER REFERENCES workflow_runs(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS background_jobs_workflow_run_id_idx
    ON background_jobs (workflow_run_id);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_run_id INTEGER NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    step_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    last_error TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT workflow_steps_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'))
);

CREATE INDEX IF NOT EXISTS workflow_steps_run_idx
    ON workflow_steps (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE IF NOT EXISTS agent_runs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_run_id INTEGER REFERENCES workflow_runs(id) ON DELETE SET NULL,
    task_id INTEGER REFERENCES case_generation_tasks(id) ON DELETE SET NULL,
    agent_name VARCHAR(96) NOT NULL,
    stage VARCHAR(96) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'running',
    input_summary TEXT,
    output_summary TEXT,
    last_error TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT agent_runs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'))
);

CREATE INDEX IF NOT EXISTS agent_runs_task_idx
    ON agent_runs (tenant_id, task_id, created_at DESC);
CREATE INDEX IF NOT EXISTS agent_runs_workflow_idx
    ON agent_runs (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE IF NOT EXISTS model_calls (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_run_id INTEGER REFERENCES workflow_runs(id) ON DELETE SET NULL,
    agent_run_id INTEGER REFERENCES agent_runs(id) ON DELETE SET NULL,
    provider VARCHAR(64),
    model VARCHAR(128),
    status VARCHAR(32) NOT NULL DEFAULT 'running',
    prompt_chars INTEGER NOT NULL DEFAULT 0,
    response_chars INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT model_calls_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT model_calls_size_check
        CHECK (prompt_chars >= 0 AND response_chars >= 0)
);

CREATE INDEX IF NOT EXISTS model_calls_agent_idx
    ON model_calls (tenant_id, agent_run_id, created_at ASC);
CREATE INDEX IF NOT EXISTS model_calls_workflow_idx
    ON model_calls (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE IF NOT EXISTS retrieval_runs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_run_id INTEGER REFERENCES workflow_runs(id) ON DELETE SET NULL,
    task_id INTEGER REFERENCES case_generation_tasks(id) ON DELETE SET NULL,
    retriever_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'succeeded',
    query_count INTEGER NOT NULL DEFAULT 0,
    hit_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT retrieval_runs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT retrieval_runs_count_check
        CHECK (query_count >= 0 AND hit_count >= 0)
);

CREATE INDEX IF NOT EXISTS retrieval_runs_task_idx
    ON retrieval_runs (tenant_id, task_id, created_at DESC);
CREATE INDEX IF NOT EXISTS retrieval_runs_workflow_idx
    ON retrieval_runs (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE IF NOT EXISTS artifacts (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_run_id INTEGER REFERENCES workflow_runs(id) ON DELETE SET NULL,
    workflow_step_id INTEGER REFERENCES workflow_steps(id) ON DELETE SET NULL,
    artifact_type VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64),
    resource_id INTEGER,
    name VARCHAR(160),
    content TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS artifacts_workflow_idx
    ON artifacts (tenant_id, workflow_run_id, created_at ASC);
CREATE INDEX IF NOT EXISTS artifacts_resource_idx
    ON artifacts (tenant_id, resource_type, resource_id, created_at DESC);

ALTER TABLE workflow_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS workflow_runs_tenant_isolation ON workflow_runs;
CREATE POLICY workflow_runs_tenant_isolation ON workflow_runs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE workflow_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_steps FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS workflow_steps_tenant_isolation ON workflow_steps;
CREATE POLICY workflow_steps_tenant_isolation ON workflow_steps
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_runs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS agent_runs_tenant_isolation ON agent_runs;
CREATE POLICY agent_runs_tenant_isolation ON agent_runs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE model_calls ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_calls FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS model_calls_tenant_isolation ON model_calls;
CREATE POLICY model_calls_tenant_isolation ON model_calls
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE retrieval_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE retrieval_runs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS retrieval_runs_tenant_isolation ON retrieval_runs;
CREATE POLICY retrieval_runs_tenant_isolation ON retrieval_runs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE artifacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE artifacts FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS artifacts_tenant_isolation ON artifacts;
CREATE POLICY artifacts_tenant_isolation ON artifacts
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
