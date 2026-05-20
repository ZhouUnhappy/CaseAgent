-- Initialize pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Tenants: 所有业务表的 owner。每个 tenant 数据相互隔离 (Phase 3 RLS 强制)。
CREATE TABLE IF NOT EXISTS tenants (
    id SERIAL PRIMARY KEY,
    slug VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create projects table
CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS projects_tenant_id_idx ON projects (tenant_id);

-- Create documents table
CREATE TABLE IF NOT EXISTS documents (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'markdown', 'gdrive'
    source VARCHAR(50) NOT NULL, -- 'upload', 'gdrive'
    content TEXT NOT NULL DEFAULT '', -- 原始 markdown 内容，供检索与重处理使用
    file_id VARCHAR(255), -- Google Drive file ID
    status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS documents_tenant_id_idx ON documents (tenant_id);

-- Create document_chunks table (tenant_id 冗余但 RLS 需要直接判定)
CREATE TABLE IF NOT EXISTS document_chunks (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    embedding vector(2000), -- 启动时会按 model.embedding.dimensions 自动校正
    parent_doc_id INTEGER, -- 用于 parent retriever
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS document_chunks_tenant_id_idx ON document_chunks (tenant_id);
CREATE INDEX IF NOT EXISTS document_chunks_embedding_idx
    ON document_chunks USING hnsw (embedding vector_cosine_ops);

-- Create knowledge_base table
CREATE TABLE IF NOT EXISTS knowledge_base (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL, -- 'product', 'module'
    name VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector(2000),
    metadata JSONB,
    status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS knowledge_base_tenant_id_idx ON knowledge_base (tenant_id);
CREATE INDEX IF NOT EXISTS knowledge_base_embedding_idx
    ON knowledge_base USING hnsw (embedding vector_cosine_ops);

-- Create case_generation_tasks table
CREATE TABLE IF NOT EXISTS case_generation_tasks (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_ids INTEGER[] NOT NULL,
    affected_products JSONB, -- ['product1', 'product2']
    affected_modules JSONB, -- ['module1', 'module2']
    status VARCHAR(50) DEFAULT 'analyzing', -- 'analyzing', 'awaiting_review', 'ready_to_generate', 'generating', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS case_generation_tasks_tenant_id_idx
    ON case_generation_tasks (tenant_id);

-- Create test_cases table
CREATE TABLE IF NOT EXISTS test_cases (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    section VARCHAR(255) NOT NULL,
    cases JSONB NOT NULL,
    source_context JSONB,
    status VARCHAR(50) DEFAULT 'draft', -- 'draft', 'submitted', 'approved'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS test_cases_tenant_id_idx ON test_cases (tenant_id);

-- Knowledge update suggestions (I4-T1)
CREATE TABLE IF NOT EXISTS knowledge_update_suggestions (
    id SERIAL PRIMARY KEY,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    candidate_type VARCHAR(32) NOT NULL, -- 'product' | 'module'
    candidate_name VARCHAR(255) NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 0,
    source_snippets JSONB,
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending' | 'adopted' | 'dismissed'
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestions_tenant_id_idx
    ON knowledge_update_suggestions (tenant_id);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestions_status_idx
    ON knowledge_update_suggestions (status);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestions_task_idx
    ON knowledge_update_suggestions (source_task_id);

-- Phase 3: Row-Level Security
-- All business tables enforce tenant_id = current_setting('app.tenant_id').
-- Application sets it via RunInTenantTx (db/tx.go) at tx start.
-- FORCE makes the table owner subject too; superusers still bypass — use a
-- NOBYPASSRLS role for runtime and integration tests.

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS projects_tenant_isolation ON projects;
CREATE POLICY projects_tenant_isolation ON projects
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS documents_tenant_isolation ON documents;
CREATE POLICY documents_tenant_isolation ON documents
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE document_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_chunks FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS document_chunks_tenant_isolation ON document_chunks;
CREATE POLICY document_chunks_tenant_isolation ON document_chunks
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE knowledge_base ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_base FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS knowledge_base_tenant_isolation ON knowledge_base;
CREATE POLICY knowledge_base_tenant_isolation ON knowledge_base
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE case_generation_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE case_generation_tasks FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS case_generation_tasks_tenant_isolation ON case_generation_tasks;
CREATE POLICY case_generation_tasks_tenant_isolation ON case_generation_tasks
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE test_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE test_cases FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS test_cases_tenant_isolation ON test_cases;
CREATE POLICY test_cases_tenant_isolation ON test_cases
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);

ALTER TABLE knowledge_update_suggestions ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_update_suggestions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS knowledge_update_suggestions_tenant_isolation ON knowledge_update_suggestions;
CREATE POLICY knowledge_update_suggestions_tenant_isolation ON knowledge_update_suggestions
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
