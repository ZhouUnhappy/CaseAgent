-- Diagnostic timestamps are instants, not tenant-local wall-clock values.
-- Legacy TIMESTAMP values are interpreted as UTC. Demo data can be rebuilt
-- with scripts/demo_bootstrap.sh fresh when historic mixed-zone rows exist.
DO $$
DECLARE
    target RECORD;
BEGIN
    FOR target IN
        SELECT *
        FROM (VALUES
            ('case_generation_tasks', 'created_at'),
            ('case_generation_tasks', 'updated_at'),
            ('background_jobs', 'run_after'),
            ('background_jobs', 'locked_at'),
            ('background_jobs', 'started_at'),
            ('background_jobs', 'finished_at'),
            ('background_jobs', 'created_at'),
            ('background_jobs', 'updated_at'),
            ('workflow_runs', 'started_at'),
            ('workflow_runs', 'finished_at'),
            ('workflow_runs', 'created_at'),
            ('workflow_runs', 'updated_at'),
            ('workflow_steps', 'started_at'),
            ('workflow_steps', 'finished_at'),
            ('workflow_steps', 'created_at'),
            ('workflow_steps', 'updated_at'),
            ('agent_runs', 'started_at'),
            ('agent_runs', 'finished_at'),
            ('agent_runs', 'created_at'),
            ('agent_runs', 'updated_at'),
            ('model_calls', 'started_at'),
            ('model_calls', 'finished_at'),
            ('model_calls', 'created_at'),
            ('model_calls', 'updated_at'),
            ('retrieval_runs', 'started_at'),
            ('retrieval_runs', 'finished_at'),
            ('retrieval_runs', 'created_at'),
            ('retrieval_runs', 'updated_at'),
            ('test_case_feedback', 'created_at'),
            ('test_case_feedback', 'updated_at')
        ) AS columns_to_convert(table_name, column_name)
    LOOP
        IF EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public'
              AND table_name = target.table_name
              AND column_name = target.column_name
              AND data_type = 'timestamp without time zone'
        ) THEN
            EXECUTE format(
                'ALTER TABLE %I ALTER COLUMN %I TYPE TIMESTAMPTZ USING %I AT TIME ZONE ''UTC''',
                target.table_name,
                target.column_name,
                target.column_name
            );
        END IF;
    END LOOP;
END $$;

-- Older rows mixed Go-bound UTC wall time (created_at) with PostgreSQL
-- CURRENT_TIMESTAMP wall time (started_at/locked_at). Repair the observed
-- eight-hour skew once; the bounded predicate keeps this statement idempotent
-- and avoids touching normally ordered rows.
DO $$
DECLARE
    tenant_row RECORD;
BEGIN
    FOR tenant_row IN SELECT id FROM tenants ORDER BY id
    LOOP
        PERFORM set_config('app.tenant_id', tenant_row.id::text, true);

        UPDATE background_jobs
        SET started_at = started_at - INTERVAL '8 hours'
        WHERE tenant_id = tenant_row.id
          AND started_at > created_at + INTERVAL '6 hours'
          AND started_at < created_at + INTERVAL '10 hours';

        UPDATE background_jobs
        SET locked_at = locked_at - INTERVAL '8 hours'
        WHERE tenant_id = tenant_row.id
          AND locked_at > created_at + INTERVAL '6 hours'
          AND locked_at < created_at + INTERVAL '10 hours';
    END LOOP;
END $$;
