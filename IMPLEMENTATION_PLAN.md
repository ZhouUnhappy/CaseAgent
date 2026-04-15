# CaseAgent 测试用例生成系统实施计划

本计划将分 6 个阶段完整实现测试用例生成系统，包括前后端、数据库、多 Agent 协同等所有功能。

## 技术栈确认

### 支持的模型（基于 eino-ext）
- **Chat Model**: openai, ark, deepseek, gemini, qianfan, qwen, openrouter, tencentcloud
- **Embedding**: openai，ark, qwen, gemini, qianfan, tencentcloud
- **Indexer**: eino-ext 未提供 pgvector indexer，需要自行实现基于 PostgreSQL + pgvector 的 indexer

### 配置方式
- 所有模型配置通过配置文件管理
- 配置文件加入 .gitignore
- 支持环境变量覆盖

### 数据库
- PostgreSQL + pgvector
- 已有数据库：eino_rag
- 账号：zhouxi
- 本地部署

### Google Drive 集成
- 后端直接调用本地 `gws drive files export` 命令
- 无需 Docker 部署

## 分阶段实施

### 阶段 1：项目初始化和数据库设计
**目标**：搭建项目基础架构，完成数据库 schema 设计

**后端**：
- 初始化 Go 项目结构（cmd, internal, pkg, configs）
- 配置 go.mod 依赖（eino, eino-ext, pgx, gin, gorm 等）
- 创建配置文件结构（config.yaml.example, config.yaml）
- 实现 .gitignore

**数据库**：
- 设计并创建 6 张表：
  - `projects`: 项目信息（id, name, description, created_at, updated_at）
  - `documents`: 文档信息（id, project_id, name, type, source, file_id, status, created_at, updated_at）
  - `document_chunks`: 文档分块（id, document_id, content, embedding vector, parent_doc_id, metadata, created_at）
  - `knowledge_base`: 知识库（id, type [product/module], name, content, embedding vector, metadata, created_at, updated_at）
  - `test_cases`: 测试用例（id, task_id, section, cases json, status, created_at, updated_at）
  - `case_generation_tasks`: 生成任务（id, project_id, document_ids, affected_products, affected_modules, status, created_at, updated_at）
- 创建 SQL migration 脚本
- 实现 pgvector 扩展初始化

**前端**：
- 初始化 Vue 3 + Vite 项目
- 安装 element-plus
- 配置路由和状态管理（pinia）
- 创建基础布局

### 阶段 2：文档处理模块
**目标**：实现文档上传、解析、分块、向量化流程

**后端**：
- 实现 PostgreSQL + pgvector Indexer（参考 eino-ext indexer 接口）
- 实现文档上传 API：
  - 支持 md 文件上传
  - 支持 Google Drive ID（调用本地 gws 命令）
- 实现文档处理流程：
  - 删除 base64 图片（正则替换）
  - Markdown HeaderSplitter 分块（## 和 ###）
  - 调用 embedding 模型向量化
  - 使用 parent indexer 存储（parent_doc_id 关联）
- 实现文档状态管理

**前端**：
- 文档上传页面
- 文档列表展示
- 文档处理状态显示

### 阶段 3：知识库管理模块
**目标**：实现架构文档的管理和向量化

**后端**：
- 实现知识库上传 API（products/modules 格式）
- 实现知识库文档处理流程（同阶段 2）
- 实现知识库查询 API
- 实现知识库更新 API

**前端**：
- 知识库管理页面
- 产品/模块文档上传
- 知识库文档列表
- 知识库文档预览

### 阶段 4：向量检索模块
**目标**：实现基于 parent retriever 的向量检索

**后端**：
- 实现 PostgreSQL + pgvector Retriever
- 实现多查询检索（multiquery）
- 实现 parent retriever（返回完整文档片段）
- 实现检索 API

**前端**：
- 检索测试页面（可选，用于调试）

### 阶段 5：多 Agent 协同模块
**目标**：实现 DeepAgent 和 4 个子 Agent

**后端**：
- 实现检索工具（retriever tool）
- 实现 4 个子 Agent：
  - 功能测试 Agent（生成功能验证用例）
  - 运维测试 Agent（生成升级、扩容、缩容用例）
  - 故障测试 Agent（生成节点重启、掉电用例）
  - 边界测试 Agent（生成参数边界、异常输入用例）
- 实现 DeepAgent（协调子 Agent）
- 实现用例生成流程：
  - 分析需求文档
  - 拆分测试类型
  - 分发任务到子 Agent
  - 汇总去重
  - 输出 JSON 用例
- 实现生成任务 API

**前端**：
- 生成任务创建页面
- 受影响产品/模块审核
- 生成进度显示
- 生成结果展示

### 阶段 6：用例审核和知识库更新
**目标**：实现用例审核和知识库更新流程

**后端**：
- 实现用例审核 API（修改、提交）
- 实现知识库更新建议 API（AI 判断是否需要更新）
- 实现知识库更新确认 API
- 完善所有 API 的错误处理和日志

**前端**：
- 用例审核页面
- 用例编辑器
- 知识库更新建议页面
- 知识库更新确认

## 项目结构

```
CaseAgent/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── api/
│   │   │   ├── handler/
│   │   │   ├── middleware/
│   │   │   └── router/
│   │   ├── config/
│   │   ├── db/
│   │   │   ├── models/
│   │   │   ├── repository/
│   │   │   └── pgvector/
│   │   ├── service/
│   │   │   ├── document/
│   │   │   ├── knowledge/
│   │   │   ├── retrieval/
│   │   │   └── agent/
│   │   └── agent/
│   │       ├── deep/
│   │       ├── functional/
│   │       ├── ops/
│   │       ├── failure/
│   │       └── boundary/
│   ├── pkg/
│   │   ├── utils/
│   │   └── gws/
│   ├── configs/
│   │   ├── config.yaml.example
│   │   └── config.yaml
│   ├── migrations/
│   │   └── *.sql
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── api/
│   │   ├── components/
│   │   ├── views/
│   │   ├── router/
│   │   ├── stores/
│   │   └── main.ts
│   ├── package.json
│   └── vite.config.ts
├── .gitignore
└── README.md
```

## 配置文件结构

```yaml
# config.yaml.example
server:
  port: 8080
  mode: debug

database:
  host: localhost
  port: 5432
  user: zhouxi
  password: your_password
  dbname: eino_rag
  sslmode: disable

model:
  chat:
    provider: openai  # openai, ark, deepseek, gemini, qianfan, qwen, openrouter, tencentcloud
    model: gpt-4
    api_key: your_api_key
    base_url: https://api.openai.com/v1
  embedding:
    provider: openai  # ark, dashscope, gemini, openai, qianfan, tencentcloud
    model: text-embedding-3-small
    api_key: your_api_key
    base_url: https://api.openai.com/v1

gws:
  enabled: true
  command: gws
```

## 数据库 Schema 详细设计

### projects
```sql
CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### documents
```sql
CREATE TABLE documents (
    id SERIAL PRIMARY KEY,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'markdown', 'gdrive'
    source VARCHAR(50) NOT NULL, -- 'upload', 'gdrive'
    file_id VARCHAR(255), -- Google Drive file ID
    status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### document_chunks
```sql
CREATE TABLE document_chunks (
    id SERIAL PRIMARY KEY,
    document_id INTEGER REFERENCES documents(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    embedding vector(1536), -- 根据模型维度调整
    parent_doc_id INTEGER, -- 用于 parent retriever
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX ON document_chunks USING ivfflat (embedding vector_cosine_ops);
```

### knowledge_base
```sql
CREATE TABLE knowledge_base (
    id SERIAL PRIMARY KEY,
    type VARCHAR(50) NOT NULL, -- 'product', 'module'
    name VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1536),
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX ON knowledge_base USING ivfflat (embedding vector_cosine_ops);
```

### test_cases
```sql
CREATE TABLE test_cases (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES case_generation_tasks(id) ON DELETE CASCADE,
    section VARCHAR(255) NOT NULL,
    cases JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'draft', -- 'draft', 'submitted', 'approved'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### case_generation_tasks
```sql
CREATE TABLE case_generation_tasks (
    id SERIAL PRIMARY KEY,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    document_ids INTEGER[] NOT NULL,
    affected_products JSONB, -- ['product1', 'product2']
    affected_modules JSONB, -- ['module1', 'module2']
    status VARCHAR(50) DEFAULT 'analyzing', -- 'analyzing', 'awaiting_review', 'generating', 'completed', 'failed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## 注意事项

1. **pgvector Indexer 实现**：需要参考 eino-ext 的 indexer 接口，自行实现基于 PostgreSQL + pgvector 的 indexer
2. **模型配置灵活性**：支持通过配置文件切换不同模型提供商
3. **错误处理**：所有 API 需要完善的错误处理和日志
4. **安全性**：配置文件包含敏感信息，必须加入 .gitignore
5. **测试**：每个阶段完成后需要进行集成测试
