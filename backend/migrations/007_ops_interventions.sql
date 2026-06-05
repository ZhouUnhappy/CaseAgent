-- Allow operators to stop queued/running jobs without deleting audit history.

ALTER TABLE background_jobs
    DROP CONSTRAINT IF EXISTS background_jobs_status_check;

ALTER TABLE background_jobs
    ADD CONSTRAINT background_jobs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'));
