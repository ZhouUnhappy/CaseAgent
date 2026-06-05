-- Track which embedding/chunking profile produced searchable vectors. Existing
-- rows intentionally default to legacy so maintenance can list and rebuild
-- them under the current runtime profile.

ALTER TABLE document_chunks
    ADD COLUMN IF NOT EXISTS index_profile VARCHAR(96) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS index_version VARCHAR(96) NOT NULL DEFAULT 'legacy';

CREATE INDEX IF NOT EXISTS document_chunks_index_profile_idx
    ON document_chunks (tenant_id, index_profile, index_version);

ALTER TABLE knowledge_base
    ADD COLUMN IF NOT EXISTS index_profile VARCHAR(96) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS index_version VARCHAR(96) NOT NULL DEFAULT 'legacy';

CREATE INDEX IF NOT EXISTS knowledge_base_index_profile_idx
    ON knowledge_base (tenant_id, index_profile, index_version);
