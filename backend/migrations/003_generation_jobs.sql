-- Persistent job runner for case generation analyze/generate work.

CREATE TABLE IF NOT EXISTS case_generation_jobs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    job_type VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 2,
    last_error TEXT,
    run_after TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMP,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT case_generation_jobs_type_check
        CHECK (job_type IN ('analyze', 'generate')),
    CONSTRAINT case_generation_jobs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    CONSTRAINT case_generation_jobs_retry_check
        CHECK (retry_count >= 0 AND max_retries >= 0)
);

CREATE INDEX IF NOT EXISTS case_generation_jobs_tenant_id_idx
    ON case_generation_jobs (tenant_id);
CREATE INDEX IF NOT EXISTS case_generation_jobs_task_id_idx
    ON case_generation_jobs (task_id);
CREATE INDEX IF NOT EXISTS case_generation_jobs_claim_idx
    ON case_generation_jobs (tenant_id, status, run_after, id)
    WHERE status = 'pending';
CREATE UNIQUE INDEX IF NOT EXISTS case_generation_jobs_active_unique_idx
    ON case_generation_jobs (tenant_id, task_id, job_type)
    WHERE status IN ('pending', 'running');

ALTER TABLE case_generation_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_generation_jobs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS case_generation_jobs_tenant_isolation ON case_generation_jobs;
CREATE POLICY case_generation_jobs_tenant_isolation ON case_generation_jobs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
