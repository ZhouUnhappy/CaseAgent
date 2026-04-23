# CaseAgent 测试用例生成系统实施计划与进度

本计划将分 6 个阶段完整实现测试用例生成系统，包括前后端、数据库、多 Agent 协同等所有功能。

## 技术栈

- **前端**: Vue 3（Vite 初始化完成）
- **后端**: Golang + Gin + Bun ORM
- **数据库**: PostgreSQL + pgvector
- **AI 框架**: eino + eino-ext
- **模型支持**: chat 当前支持 Ark；embedding 当前支持 Ark / OpenAI-compatible

### 当前支持的模型
- **Chat Model**: ark
- **Embedding**: ark / openai-compatible
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

## 当前状态（2026-04-23）

### 已实现功能
- 完整的项目结构
- 数据库 Schema 和迁移脚本
- 所有基础 API（项目、文档、知识库、任务、测试用例）
- 文档处理框架（支持文件上传和 Google Drive）
- pgvector indexer 和 retriever 框架
- Agent 骨架（DeepAgent + 4 个子 Agent）
- HTTP 服务器（Gin）

### 各阶段完成情况
- **阶段 1**：后端项目结构、数据库 schema、迁移脚本已完成；前端仅完成 Vue 3 + Vite 初始化，`element-plus`、`pinia`、路由和基础布局未开始。
- **阶段 2**：文档上传和异步处理框架已落地；文档清洗、分块、embedding 存储可继续完善，parent retriever 闭环尚未完成。
- **阶段 3**：知识库 CRUD 已完成；知识库向量化先按"整篇文档一个 embedding"落地，后续如需细粒度检索再扩展 chunk 级设计。
- **阶段 4**：pgvector 检索框架与基础检索 API 已落地；多查询检索、parent retriever 深化能力仍待补齐。
- **阶段 5**：4 个子 Agent 与服务层第一版协调已落地；更复杂的多 Agent 协同与任务分发未完成。
- **阶段 6**：测试用例审核 API 已有基础接口；知识库更新建议/确认流程仍待实现。

### 当前联调阻塞点
- mixed provider 已恢复：`chat=ark`、`embedding=openai-compatible` 可以编译运行。
- 真实接口联调已暴露两个待修问题：
  1. `KnowledgeBase` 模型的表名映射与 migration 不一致，当前会访问 `knowledge_bases`，而数据库实际表名是 `knowledge_base`。
  2. pgvector 列当前直接写 `[]float32` 失败，文档 chunk 向量落库时报 `bufio: buffer full`，需要补正确的 vector 类型或编码方式。

### 待实现细节
以下能力仍需后续完善：
- 多查询检索与更完整的 parent retriever
- DeepAgent 的实际协调逻辑
- 子 Agent 的进一步去重与协同逻辑
- 前端页面实现
- 当前两处运行时阻塞修复
- 完整端到端联调与真实模型调试

## 后续执行顺序

1. 先修复当前联调阻塞点，打通"文档上传 + 知识库上传 + 检索"真实链路。
2. 再补检索增强：完成 parent retriever、多查询检索、知识库与需求拼装。
3. 最后继续做多 Agent 协同和前端页面。

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
    provider: ark
    model: ep-your-chat-endpoint
    api_key: your_ark_api_key
    access_key: ""
    secret_key: ""
    base_url: https://ark.cn-beijing.volces.com/api/v3
    region: cn-beijing
  embedding:
    provider: ark
    model: ep-your-embedding-endpoint
    api_key: your_ark_api_key
    access_key: ""
    secret_key: ""
    base_url: https://ark.cn-beijing.volces.com/api/v3
    region: cn-beijing

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

## API 端点

- `POST /api/v1/projects` - 创建项目
- `GET /api/v1/projects` - 列出项目
- `GET /api/v1/projects/:id` - 获取项目详情
- `PUT /api/v1/projects/:id` - 更新项目
- `DELETE /api/v1/projects/:id` - 删除项目
- `POST /api/v1/projects/:id/documents` - 上传文档
- `GET /api/v1/projects/:id/documents` - 列出文档
- `GET /api/v1/documents/:id` - 获取文档详情
- `DELETE /api/v1/documents/:id` - 删除文档
- `POST /api/v1/knowledge` - 上传知识库
- `GET /api/v1/knowledge` - 列出知识库
- `GET /api/v1/knowledge/:id` - 获取知识库详情
- `PUT /api/v1/knowledge/:id` - 更新知识库
- `DELETE /api/v1/knowledge/:id` - 删除知识库
- `POST /api/v1/projects/:id/tasks` - 创建生成任务
- `GET /api/v1/projects/:id/tasks` - 列出任务
- `GET /api/v1/tasks/:id` - 获取任务详情
- `PUT /api/v1/tasks/:id/review` - 审核受影响产品/模块
- `PUT /api/v1/tasks/:id/generate` - 生成测试用例
- `GET /api/v1/tasks/:id/cases` - 获取测试用例
- `PUT /api/v1/tasks/:id/cases/:case_id` - 更新测试用例
- `PUT /api/v1/tasks/:id/cases/:case_id/submit` - 提交测试用例

## 配置说明

配置文件位于 `backend/configs/config.yaml`，包含：
- 服务器配置（端口、模式）
- 数据库配置（PostgreSQL + pgvector）
- 模型配置（Chat Model 和 Embedding）
- Google Drive 配置（gws 命令）

**注意**: `config.yaml` 包含敏感信息，已加入 .gitignore。使用前请复制 `config.yaml.example` 并填写实际配置。

## 注意事项

1. **pgvector Indexer 实现**：需要参考 eino-ext 的 indexer 接口，自行实现基于 PostgreSQL + pgvector 的 indexer
2. **模型配置灵活性**：支持通过配置文件切换不同模型提供商
3. **错误处理**：所有 API 需要完善的错误处理和日志
4. **安全性**：配置文件包含敏感信息，必须加入 .gitignore
5. **测试**：每个阶段完成后需要进行集成测试
