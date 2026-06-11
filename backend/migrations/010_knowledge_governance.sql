-- Add lightweight governance fields for knowledge lifecycle and operator review.

ALTER TABLE knowledge_base
    ADD COLUMN IF NOT EXISTS source VARCHAR(64) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS duplicate_of_id INTEGER REFERENCES knowledge_base(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS duplicate_marked_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS knowledge_base_source_idx
    ON knowledge_base (tenant_id, source);

CREATE INDEX IF NOT EXISTS knowledge_base_expires_at_idx
    ON knowledge_base (tenant_id, expires_at)
    WHERE expires_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS knowledge_base_duplicate_idx
    ON knowledge_base (tenant_id, duplicate_of_id)
    WHERE duplicate_of_id IS NOT NULL;
