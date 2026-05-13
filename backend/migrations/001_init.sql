-- Initialize pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Create projects table
CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create documents table
CREATE TABLE IF NOT EXISTS documents (
    id SERIAL PRIMARY KEY,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'markdown', 'gdrive'
    source VARCHAR(50) NOT NULL, -- 'upload', 'gdrive'
    content TEXT NOT NULL DEFAULT '', -- 原始 markdown 内容，供检索与重处理使用
    file_id VARCHAR(255), -- Google Drive file ID
    status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Backfill columns added after early local schemas already existed.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS content TEXT NOT NULL DEFAULT '';

-- Create document_chunks table
CREATE TABLE IF NOT EXISTS document_chunks (
    id SERIAL PRIMARY KEY,
    document_id INTEGER REFERENCES documents(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    embedding vector(2000), -- 启动时会按 model.embedding.dimensions 自动校正
    parent_doc_id INTEGER, -- 用于 parent retriever
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for document_chunks embedding
CREATE INDEX IF NOT EXISTS document_chunks_embedding_idx ON document_chunks USING ivfflat (embedding vector_cosine_ops);

-- Create knowledge_base table
CREATE TABLE IF NOT EXISTS knowledge_base (
    id SERIAL PRIMARY KEY,
    type VARCHAR(50) NOT NULL, -- 'product', 'module'
    name VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector(2000),
    metadata JSONB,
    status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Backfill columns added after early local schemas already existed.
ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS metadata JSONB;
ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'pending';
ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

-- Create index for knowledge_base embedding
CREATE INDEX IF NOT EXISTS knowledge_base_embedding_idx ON knowledge_base USING ivfflat (embedding vector_cosine_ops);

-- Create case_generation_tasks table
CREATE TABLE IF NOT EXISTS case_generation_tasks (
    id SERIAL PRIMARY KEY,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    document_ids INTEGER[] NOT NULL,
    affected_products JSONB, -- ['product1', 'product2']
    affected_modules JSONB, -- ['module1', 'module2']
    status VARCHAR(50) DEFAULT 'analyzing', -- 'analyzing', 'awaiting_review', 'ready_to_generate', 'generating', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create test_cases table
CREATE TABLE IF NOT EXISTS test_cases (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    section VARCHAR(255) NOT NULL,
    cases JSONB NOT NULL,
    source_context JSONB,
    status VARCHAR(50) DEFAULT 'draft', -- 'draft', 'submitted', 'approved'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Backfill columns added after early local schemas already existed.
ALTER TABLE test_cases ADD COLUMN IF NOT EXISTS source_context JSONB;

-- Knowledge update suggestions (I4-T1)
CREATE TABLE IF NOT EXISTS knowledge_update_suggestions (
    id SERIAL PRIMARY KEY,
    source_task_id INTEGER NOT NULL REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    candidate_type VARCHAR(32) NOT NULL, -- 'product' | 'module'
    candidate_name VARCHAR(255) NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 0,
    source_snippets JSONB,
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending' | 'adopted' | 'dismissed'
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS knowledge_update_suggestions_status_idx
    ON knowledge_update_suggestions (status);
CREATE INDEX IF NOT EXISTS knowledge_update_suggestions_task_idx
    ON knowledge_update_suggestions (source_task_id);
