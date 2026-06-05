-- Extend the persistent job table beyond case generation work.

ALTER TABLE case_generation_jobs
    ALTER COLUMN task_id DROP NOT NULL;

ALTER TABLE case_generation_jobs
    ADD COLUMN IF NOT EXISTS document_id INTEGER REFERENCES documents(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS knowledge_id INTEGER REFERENCES knowledge_base(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE case_generation_jobs
    DROP CONSTRAINT IF EXISTS case_generation_jobs_type_check;

ALTER TABLE case_generation_jobs
    ADD CONSTRAINT case_generation_jobs_type_check
        CHECK (job_type IN (
            'analyze',
            'generate',
            'document_process',
            'document_reprocess',
            'knowledge_process',
            'knowledge_reprocess'
        ));

ALTER TABLE case_generation_jobs
    DROP CONSTRAINT IF EXISTS case_generation_jobs_resource_check;

ALTER TABLE case_generation_jobs
    ADD CONSTRAINT case_generation_jobs_resource_check
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
        );

CREATE INDEX IF NOT EXISTS case_generation_jobs_document_id_idx
    ON case_generation_jobs (document_id);
CREATE INDEX IF NOT EXISTS case_generation_jobs_knowledge_id_idx
    ON case_generation_jobs (knowledge_id);
CREATE INDEX IF NOT EXISTS case_generation_jobs_type_claim_idx
    ON case_generation_jobs (tenant_id, job_type, status, run_after, id)
    WHERE status = 'pending';

DROP INDEX IF EXISTS case_generation_jobs_active_unique_idx;

CREATE UNIQUE INDEX IF NOT EXISTS case_generation_jobs_active_task_unique_idx
    ON case_generation_jobs (tenant_id, task_id, job_type)
    WHERE status IN ('pending', 'running') AND task_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS case_generation_jobs_active_document_unique_idx
    ON case_generation_jobs (tenant_id, document_id, job_type)
    WHERE status IN ('pending', 'running') AND document_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS case_generation_jobs_active_knowledge_unique_idx
    ON case_generation_jobs (tenant_id, knowledge_id, job_type)
    WHERE status IN ('pending', 'running') AND knowledge_id IS NOT NULL;
