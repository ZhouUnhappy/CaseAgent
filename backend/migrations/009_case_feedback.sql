-- Case-level human feedback. This is intentionally plain relational data
-- plus JSONB source summaries so frontend/backend engineers can build the
-- quality loop without introducing algorithm or data-platform dependencies.

CREATE TABLE IF NOT EXISTS test_case_feedback (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    test_case_id INTEGER NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    case_index INTEGER NOT NULL,
    case_title TEXT,
    feedback_type VARCHAR(64) NOT NULL,
    note TEXT,
    source_context_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    prompt_id VARCHAR(128),
    prompt_version VARCHAR(64),
    model_call_id INTEGER REFERENCES model_calls(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT test_case_feedback_case_index_check
        CHECK (case_index >= 0),
    CONSTRAINT test_case_feedback_type_check
        CHECK (feedback_type IN ('useful', 'duplicate', 'missing_steps', 'requirement_mismatch', 'knowledge_missing'))
);

CREATE INDEX IF NOT EXISTS test_case_feedback_task_idx
    ON test_case_feedback (tenant_id, task_id, created_at DESC);
CREATE INDEX IF NOT EXISTS test_case_feedback_case_idx
    ON test_case_feedback (tenant_id, test_case_id, case_index, created_at DESC);
CREATE INDEX IF NOT EXISTS test_case_feedback_type_idx
    ON test_case_feedback (tenant_id, feedback_type, created_at DESC);
CREATE INDEX IF NOT EXISTS test_case_feedback_model_call_idx
    ON test_case_feedback (tenant_id, model_call_id)
    WHERE model_call_id IS NOT NULL;

ALTER TABLE test_case_feedback ENABLE ROW LEVEL SECURITY;
ALTER TABLE test_case_feedback FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS test_case_feedback_tenant_isolation ON test_case_feedback;
CREATE POLICY test_case_feedback_tenant_isolation ON test_case_feedback
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
