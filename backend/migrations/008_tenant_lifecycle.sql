-- Tenant lifecycle for demo/productization: archived tenants stay auditable
-- but are hidden from normal selection and rejected by business APIs.

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS tenants_archived_at_idx
    ON tenants (archived_at);
