-- P4: aggregate repeated knowledge suggestions across tasks.

CREATE TABLE IF NOT EXISTS knowledge_update_suggestion_groups (
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
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_groups_tenant_id_idx
    ON knowledge_update_suggestion_groups (tenant_id);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_groups_status_idx
    ON knowledge_update_suggestion_groups (status);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_groups_priority_idx
    ON knowledge_update_suggestion_groups (task_count DESC, total_frequency DESC, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_groups_resolved_knowledge_idx
    ON knowledge_update_suggestion_groups (resolved_knowledge_id);

CREATE TABLE IF NOT EXISTS knowledge_update_suggestion_occurrences (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL REFERENCES knowledge_update_suggestion_groups(id) ON DELETE CASCADE,
    source_task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    source_case_id INTEGER REFERENCES test_cases(id) ON DELETE SET NULL,
    frequency INTEGER NOT NULL DEFAULT 0,
    source_snippets JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_occurrences_tenant_id_idx
    ON knowledge_update_suggestion_occurrences (tenant_id);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_occurrences_group_idx
    ON knowledge_update_suggestion_occurrences (group_id);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_occurrences_task_idx
    ON knowledge_update_suggestion_occurrences (source_task_id);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestion_occurrences_case_idx
    ON knowledge_update_suggestion_occurrences (source_case_id);
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_update_suggestion_occurrences_source_unique_idx
    ON knowledge_update_suggestion_occurrences (
        tenant_id,
        group_id,
        source_task_id,
        COALESCE(source_case_id, 0)
    );

ALTER TABLE knowledge_update_suggestion_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_update_suggestion_groups FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS knowledge_update_suggestion_groups_tenant_isolation ON knowledge_update_suggestion_groups;
CREATE POLICY knowledge_update_suggestion_groups_tenant_isolation ON knowledge_update_suggestion_groups
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE knowledge_update_suggestion_occurrences ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_update_suggestion_occurrences FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS knowledge_update_suggestion_occurrences_tenant_isolation ON knowledge_update_suggestion_occurrences;
CREATE POLICY knowledge_update_suggestion_occurrences_tenant_isolation ON knowledge_update_suggestion_occurrences
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
