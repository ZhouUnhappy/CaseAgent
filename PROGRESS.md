# CaseAgent 实施进度总结

## 已完成工作

### 阶段 1: 后端与数据库基础完成，前端初始化完成

**后端**:
- 初始化 Go 项目结构
- 配置 go.mod 依赖
- 创建配置文件结构
- 实现 .gitignore
- 支持配置文件 + 环境变量覆盖

**数据库**:
- 设计并创建 6 张表
- 创建 SQL migration 脚本
- 实现 pgvector 扩展初始化

**前端**:
- 已初始化 Vue 3 + Vite 项目
- `element-plus`、`pinia`、路由、基础布局仍未实现

### 阶段 2: 文档处理框架已落地，仍需补全闭环

**后端**:
- 实现 PostgreSQL + pgvector Indexer（简化版）
- 实现文档上传 API（支持 md 文件上传和 Google Drive ID）
- 实现文档处理流程：
  - 删除 base64 图片
  - 基于标题的简化分块
  - 按 provider 初始化 embedding 存储（当前支持 Ark / OpenAI-compatible）
  - `parent_doc_id` 基础关联
- parent retriever 仍未完成

### 阶段 3: 知识库 CRUD 已完成，向量化为简化实现

**后端**:
- 实现知识库上传 API
- 实现知识库 embedding 更新流程
- 实现知识库查询、更新、删除 API

### 阶段 4: 检索框架已落地

**后端**:
- 实现 PostgreSQL + pgvector Retriever
- 实现向量相似度检索框架
- 已实现基础检索 API：
  - 文档检索接口
  - 知识库检索接口
  - 文档结果按 parent document 聚合返回
- 多查询检索仍未完成

### 阶段 5: 单 Agent 主链路已打通，多 Agent 已有第一版协调

**后端**:
- 实现 4 个子 Agent prompt 骨架：
  - 功能测试 Agent
  - 运维测试 Agent
  - 故障测试 Agent
  - 边界测试 Agent
- 服务层已协调 4 个子 Agent 分别生成 section 并合并结果
- DeepAgent 仍作为汇总回退路径
- 已打通生成任务主链路：
  - 创建任务后异步分析受影响 products/modules
  - 审核后可触发异步生成
  - 生成结果已按 section 落库到 `test_cases`
- 更复杂的任务分发、并行协同、汇总去重仍未完成

### 阶段 6: 审核接口部分完成

**后端**:
- 实现用例审核 API（修改、提交）
- 基础错误处理已覆盖主要 handler
- 知识库更新建议/确认流程仍未实现

## 当前状态

### 已实现功能
- 完整的项目结构
- 数据库 Schema 和迁移脚本
- 所有基础 API（项目、文档、知识库、任务、测试用例）
- 文档处理框架（支持文件上传和 Google Drive）
- pgvector indexer 和 retriever 框架
- Agent 骨架（DeepAgent + 4 个子 Agent）
- HTTP 服务器（Gin）

### 当前联调状态
- `chat=ark`、`embedding=ark/openai-compatible` 的 mixed provider 代码路径已恢复。
- 后端静态验证通过：`go test ./...` 通过。
- 真实接口联调已确认服务、数据库、真实 key 可用。
- 当前仍有两个运行时阻塞：
  - `knowledge_base` 表名映射错误，知识库接口会访问 `knowledge_bases`
  - pgvector 写入方式不正确，文档 chunk embedding 落库失败并报 `bufio: buffer full`

### 待实现细节
以下能力仍需后续完善：
- 多查询检索与更完整的 parent retriever
- DeepAgent 的实际协调逻辑
- 子 Agent 的进一步去重与协同逻辑
- 前端页面实现
- 当前两处运行时阻塞修复
- 完整端到端联调与真实模型调试

## 技术栈

- **前端**: Vue 3（Vite 初始化完成）
- **后端**: Golang + Gin + Bun ORM
- **数据库**: PostgreSQL + pgvector
- **AI 框架**: eino + eino-ext
- **模型支持**: chat 当前支持 Ark；embedding 当前支持 Ark / OpenAI-compatible

## 下一步建议

1. 完成 parent retriever 与多查询检索
2. 修复知识库表名映射与 pgvector 写入问题
3. 完成真实的多 Agent 协同
4. 实现前端页面与 API 集成

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
