CREATE EXTENSION vector;

CREATE TABLE caseagent_schema (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    schema_hash VARCHAR(64) NOT NULL,
    embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions > 0),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tenants (
    id SERIAL PRIMARY KEY,
    slug VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    archived_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX tenants_archived_at_idx ON tenants (archived_at);

CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX projects_tenant_id_idx ON projects (tenant_id);

CREATE TABLE documents (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    source VARCHAR(50) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    file_id VARCHAR(255),
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX documents_tenant_id_idx ON documents (tenant_id);

CREATE TABLE document_chunks (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    embedding vector({{EMBEDDING_DIMENSIONS}}),
    parent_doc_id INTEGER,
    metadata JSONB,
    index_profile VARCHAR(96) NOT NULL,
    index_version VARCHAR(96) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX document_chunks_tenant_id_idx ON document_chunks (tenant_id);
CREATE INDEX document_chunks_embedding_idx
    ON document_chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX document_chunks_index_profile_idx
    ON document_chunks (tenant_id, index_profile, index_version);

CREATE TABLE knowledge_base (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector({{EMBEDDING_DIMENSIONS}}),
    metadata JSONB,
    source VARCHAR(64) NOT NULL DEFAULT 'manual',
    expires_at TIMESTAMP,
    duplicate_of_id INTEGER REFERENCES knowledge_base(id) ON DELETE SET NULL,
    duplicate_marked_at TIMESTAMP,
    index_profile VARCHAR(96) NOT NULL,
    index_version VARCHAR(96) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX knowledge_base_tenant_id_idx ON knowledge_base (tenant_id);
CREATE INDEX knowledge_base_embedding_idx
    ON knowledge_base USING hnsw (embedding vector_cosine_ops);
CREATE INDEX knowledge_base_index_profile_idx
    ON knowledge_base (tenant_id, index_profile, index_version);
CREATE INDEX knowledge_base_source_idx
    ON knowledge_base (tenant_id, source);
CREATE INDEX knowledge_base_expires_at_idx
    ON knowledge_base (tenant_id, expires_at)
    WHERE expires_at IS NOT NULL;
CREATE INDEX knowledge_base_duplicate_idx
    ON knowledge_base (tenant_id, duplicate_of_id)
    WHERE duplicate_of_id IS NOT NULL;

CREATE TABLE case_generation_tasks (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_ids INTEGER[] NOT NULL,
    affected_products JSONB,
    affected_modules JSONB,
    status VARCHAR(50) DEFAULT 'analyzing',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX case_generation_tasks_tenant_id_idx
    ON case_generation_tasks (tenant_id);

CREATE TABLE test_cases (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    section VARCHAR(255) NOT NULL,
    cases JSONB NOT NULL,
    source_context JSONB,
    status VARCHAR(50) DEFAULT 'draft',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX test_cases_tenant_id_idx ON test_cases (tenant_id);

CREATE TABLE knowledge_update_suggestion_groups (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    candidate_type VARCHAR(32) NOT NULL,
    candidate_name VARCHAR(255) NOT NULL,
    total_frequency INTEGER NOT NULL DEFAULT 0,
    task_count INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    dismissed_reason VARCHAR(64),
    resolved_knowledge_id INTEGER REFERENCES knowledge_base(id) ON DELETE SET NULL,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT knowledge_update_suggestion_groups_unique_candidate
        UNIQUE (tenant_id, candidate_type, candidate_name)
);
CREATE INDEX knowledge_update_suggestion_groups_tenant_id_idx
    ON knowledge_update_suggestion_groups (tenant_id);
CREATE INDEX knowledge_update_suggestion_groups_status_idx
    ON knowledge_update_suggestion_groups (status);
CREATE INDEX knowledge_update_suggestion_groups_priority_idx
    ON knowledge_update_suggestion_groups (task_count DESC, total_frequency DESC, last_seen_at DESC);
CREATE INDEX knowledge_update_suggestion_groups_resolved_knowledge_idx
    ON knowledge_update_suggestion_groups (resolved_knowledge_id);

CREATE TABLE knowledge_update_suggestion_occurrences (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL REFERENCES knowledge_update_suggestion_groups(id) ON DELETE CASCADE,
    source_task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    source_case_id INTEGER REFERENCES test_cases(id) ON DELETE SET NULL,
    frequency INTEGER NOT NULL DEFAULT 0,
    source_snippets JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX knowledge_update_suggestion_occurrences_tenant_id_idx
    ON knowledge_update_suggestion_occurrences (tenant_id);
CREATE INDEX knowledge_update_suggestion_occurrences_group_idx
    ON knowledge_update_suggestion_occurrences (group_id);
CREATE INDEX knowledge_update_suggestion_occurrences_task_idx
    ON knowledge_update_suggestion_occurrences (source_task_id);
CREATE INDEX knowledge_update_suggestion_occurrences_case_idx
    ON knowledge_update_suggestion_occurrences (source_case_id);
CREATE UNIQUE INDEX knowledge_update_suggestion_occurrences_source_unique_idx
    ON knowledge_update_suggestion_occurrences (
        tenant_id,
        group_id,
        source_task_id,
        COALESCE(source_case_id, 0)
    );

CREATE TABLE background_jobs (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id INTEGER REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    document_id INTEGER REFERENCES documents(id) ON DELETE CASCADE,
    knowledge_id INTEGER REFERENCES knowledge_base(id) ON DELETE CASCADE,
    workflow_run_id INTEGER,
    job_type VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 2,
    last_error TEXT,
    run_after TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT background_jobs_type_check
        CHECK (job_type IN (
            'analyze',
            'generate',
            'document_process',
            'document_reprocess',
            'knowledge_process',
            'knowledge_reprocess'
        )),
    CONSTRAINT background_jobs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT background_jobs_retry_check
        CHECK (retry_count >= 0 AND max_retries >= 0),
    CONSTRAINT background_jobs_resource_check
        CHECK (
            (job_type IN ('analyze', 'generate')
                AND task_id IS NOT NULL
                AND document_id IS NULL
                AND knowledge_id IS NULL)
            OR (job_type IN ('document_process', 'document_reprocess')
                AND task_id IS NULL
                AND document_id IS NOT NULL
                AND knowledge_id IS NULL)
            OR (job_type IN ('knowledge_process', 'knowledge_reprocess')
                AND task_id IS NULL
                AND document_id IS NULL
                AND knowledge_id IS NOT NULL)
        )
);
CREATE INDEX background_jobs_tenant_id_idx
    ON background_jobs (tenant_id);
CREATE INDEX background_jobs_task_id_idx
    ON background_jobs (task_id);
CREATE INDEX background_jobs_document_id_idx
    ON background_jobs (document_id);
CREATE INDEX background_jobs_knowledge_id_idx
    ON background_jobs (knowledge_id);
CREATE INDEX background_jobs_claim_idx
    ON background_jobs (tenant_id, status, run_after, id)
    WHERE status = 'pending';
CREATE INDEX background_jobs_type_claim_idx
    ON background_jobs (tenant_id, job_type, status, run_after, id)
    WHERE status = 'pending';
CREATE UNIQUE INDEX background_jobs_active_task_unique_idx
    ON background_jobs (tenant_id, task_id, job_type)
    WHERE status IN ('pending', 'running') AND task_id IS NOT NULL;
CREATE UNIQUE INDEX background_jobs_active_document_unique_idx
    ON background_jobs (tenant_id, document_id, job_type)
    WHERE status IN ('pending', 'running') AND document_id IS NOT NULL;
CREATE UNIQUE INDEX background_jobs_active_knowledge_unique_idx
    ON background_jobs (tenant_id, knowledge_id, job_type)
    WHERE status IN ('pending', 'running') AND knowledge_id IS NOT NULL;

CREATE TABLE workflow_runs (
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
CREATE INDEX workflow_runs_tenant_resource_idx
    ON workflow_runs (tenant_id, resource_type, resource_id, created_at DESC);
CREATE INDEX workflow_runs_job_id_idx ON workflow_runs (job_id);
CREATE INDEX workflow_runs_status_idx
    ON workflow_runs (tenant_id, status, created_at DESC);

ALTER TABLE background_jobs
    ADD CONSTRAINT background_jobs_workflow_run_id_fkey
    FOREIGN KEY (workflow_run_id) REFERENCES workflow_runs(id) ON DELETE SET NULL;
CREATE INDEX background_jobs_workflow_run_id_idx
    ON background_jobs (workflow_run_id);

CREATE TABLE workflow_steps (
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
CREATE INDEX workflow_steps_run_idx
    ON workflow_steps (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE agent_runs (
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
CREATE INDEX agent_runs_task_idx
    ON agent_runs (tenant_id, task_id, created_at DESC);
CREATE INDEX agent_runs_workflow_idx
    ON agent_runs (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE model_calls (
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
CREATE INDEX model_calls_agent_idx
    ON model_calls (tenant_id, agent_run_id, created_at ASC);
CREATE INDEX model_calls_workflow_idx
    ON model_calls (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE retrieval_runs (
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
CREATE INDEX retrieval_runs_task_idx
    ON retrieval_runs (tenant_id, task_id, created_at DESC);
CREATE INDEX retrieval_runs_workflow_idx
    ON retrieval_runs (tenant_id, workflow_run_id, created_at ASC);

CREATE TABLE artifacts (
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
CREATE INDEX artifacts_workflow_idx
    ON artifacts (tenant_id, workflow_run_id, created_at ASC);
CREATE INDEX artifacts_resource_idx
    ON artifacts (tenant_id, resource_type, resource_id, created_at DESC);

CREATE TABLE test_case_feedback (
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT test_case_feedback_case_index_check
        CHECK (case_index >= 0),
    CONSTRAINT test_case_feedback_type_check
        CHECK (feedback_type IN (
            'useful',
            'duplicate',
            'missing_steps',
            'requirement_mismatch',
            'knowledge_missing'
        ))
);
CREATE INDEX test_case_feedback_task_idx
    ON test_case_feedback (tenant_id, task_id, created_at DESC);
CREATE INDEX test_case_feedback_case_idx
    ON test_case_feedback (tenant_id, test_case_id, case_index, created_at DESC);
CREATE INDEX test_case_feedback_type_idx
    ON test_case_feedback (tenant_id, feedback_type, created_at DESC);
CREATE INDEX test_case_feedback_model_call_idx
    ON test_case_feedback (tenant_id, model_call_id)
    WHERE model_call_id IS NOT NULL;

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects FORCE ROW LEVEL SECURITY;
CREATE POLICY projects_tenant_isolation ON projects
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents FORCE ROW LEVEL SECURITY;
CREATE POLICY documents_tenant_isolation ON documents
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE document_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_chunks FORCE ROW LEVEL SECURITY;
CREATE POLICY document_chunks_tenant_isolation ON document_chunks
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE knowledge_base ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_base FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_base_tenant_isolation ON knowledge_base
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE case_generation_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_generation_tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY case_generation_tasks_tenant_isolation ON case_generation_tasks
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE test_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE test_cases FORCE ROW LEVEL SECURITY;
CREATE POLICY test_cases_tenant_isolation ON test_cases
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE knowledge_update_suggestion_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_update_suggestion_groups FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_update_suggestion_groups_tenant_isolation
    ON knowledge_update_suggestion_groups
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE knowledge_update_suggestion_occurrences ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_update_suggestion_occurrences FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_update_suggestion_occurrences_tenant_isolation
    ON knowledge_update_suggestion_occurrences
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE background_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE background_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY background_jobs_tenant_isolation ON background_jobs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE workflow_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY workflow_runs_tenant_isolation ON workflow_runs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE workflow_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_steps FORCE ROW LEVEL SECURITY;
CREATE POLICY workflow_steps_tenant_isolation ON workflow_steps
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY agent_runs_tenant_isolation ON agent_runs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE model_calls ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_calls FORCE ROW LEVEL SECURITY;
CREATE POLICY model_calls_tenant_isolation ON model_calls
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE retrieval_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE retrieval_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY retrieval_runs_tenant_isolation ON retrieval_runs
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE artifacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE artifacts FORCE ROW LEVEL SECURITY;
CREATE POLICY artifacts_tenant_isolation ON artifacts
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE test_case_feedback ENABLE ROW LEVEL SECURITY;
ALTER TABLE test_case_feedback FORCE ROW LEVEL SECURITY;
CREATE POLICY test_case_feedback_tenant_isolation ON test_case_feedback
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
