# CaseAgent 实施进度总结

## 已完成工作

### 阶段 1: 项目初始化和数据库设计 ✓

**后端**:
- 初始化 Go 项目结构
- 配置 go.mod 依赖
- 创建配置文件结构
- 实现 .gitignore

**数据库**:
- 设计并创建 6 张表
- 创建 SQL migration 脚本
- 实现 pgvector 扩展初始化

**前端**:
- 用户已自行创建 Vue 3 + Vite 项目

### 阶段 2: 文档处理模块 ✓

**后端**:
- 实现 PostgreSQL + pgvector Indexer（简化版）
- 实现文档上传 API（支持 md 文件上传和 Google Drive ID）
- 实现文档处理流程：
  - 删除 base64 图片
  - Markdown HeaderSplitter 分块
  - 向量化存储框架
  - parent indexer 存储框架

### 阶段 3: 知识库管理模块 ✓

**后端**:
- 实现知识库上传 API
- 实现知识库文档处理流程
- 实现知识库查询、更新、删除 API

### 阶段 4: 向量检索模块 ✓

**后端**:
- 实现 PostgreSQL + pgvector Retriever
- 实现向量相似度检索框架

### 阶段 5: 多 Agent 协同模块 ✓

**后端**:
- 实现检索工具框架
- 实现 4 个子 Agent 框架：
  - 功能测试 Agent
  - 运维测试 Agent
  - 故障测试 Agent
  - 边界测试 Agent
- 实现 DeepAgent 协调框架
- 实现用例生成流程框架

### 阶段 6: 用例审核和知识库更新 ✓

**后端**:
- 实现用例审核 API（修改、提交）
- 实现知识库更新建议框架
- 完善所有 API 的错误处理

## 当前状态

### 已实现功能
- 完整的项目结构
- 数据库 Schema 和迁移脚本
- 所有基础 API（项目、文档、知识库、任务、测试用例）
- 文档处理框架（支持文件上传和 Google Drive）
- pgvector indexer 和 retriever 框架
- 多 Agent 协同框架（DeepAgent + 4个子Agent）
- HTTP 服务器（Gin）

### 待实现细节
由于 eino 和 eino-ext 的包依赖问题，以下功能需要后续完善：
- 实际的 embedding 模型调用
- 完整的文档分块和向量化流程
- DeepAgent 的实际协调逻辑
- 子 Agent 的实际测试用例生成逻辑
- 向量检索的实际集成
- 前端页面实现

## 技术栈

- **前端**: Vue 3 + Element Plus（用户已创建）
- **后端**: Golang + Gin + Bun ORM
- **数据库**: PostgreSQL + pgvector
- **AI 框架**: eino + eino-ext（框架已集成，具体实现待完善）

## 支持的模型

- **Chat Model**: openai, ark, deepseek, gemini, qianfan, qwen, openrouter, tencentcloud
- **Embedding**: ark, dashscope, gemini, openai, qianfan, tencentcloud

## 下一步建议

1. **解决 eino 依赖问题**：需要正确配置 eino 和 eino-ext 的本地路径或使用远程版本
2. **实现 embedding 集成**：选择并配置 embedding 模型
3. **完善文档处理**：实现完整的文档分块、向量化、存储流程
4. **实现 Agent 逻辑**：完善 DeepAgent 和子 Agent 的实际生成逻辑
5. **前端开发**：实现前端页面和 API 集成
6. **测试和调试**：端到端测试整个流程

## 项目结构

```
CaseAgent/
├── backend/
│   ├── cmd/server/           # 服务器入口
│   ├── internal/
│   │   ├── api/              # API 层
│   │   │   ├── handler/      # HTTP 处理器
│   │   │   ├── router/       # 路由配置
│   │   ├── config/           # 配置管理
│   │   ├── db/               # 数据库
│   │   │   ├── models/       # 数据模型
│   │   │   ├── pgvector/     # pgvector indexer/retriever
│   │   ├── service/          # 业务逻辑
│   │   │   ├── document/     # 文档处理
│   │   │   ├── knowledge/    # 知识库管理
│   │   │   ├── agent/        # Agent 协同
│   │   ├── agent/            # Agent 实现
│   │   │   ├── deep/         # DeepAgent
│   │   │   ├── functional/   # 功能测试 Agent
│   │   │   ├── ops/          # 运维测试 Agent
│   │   │   ├── failure/      # 故障测试 Agent
│   │   │   └── boundary/     # 边界测试 Agent
│   ├── configs/              # 配置文件
│   ├── migrations/           # 数据库迁移
│   └── go.mod
├── frontend/                 # Vue 前端（用户已创建）
├── IMPLEMENTATION_PLAN.md    # 实施计划
├── PROGRESS.md              # 进度总结
└── README.md                # 项目说明
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
