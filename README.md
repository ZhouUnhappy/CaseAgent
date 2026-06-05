# CaseAgent

测试用例生成系统 - 基于 AI 的自动化测试用例生成工具

## 技术栈

- **前端**: Vue 3 + Element Plus
- **后端**: Golang + Gin + eino + eino-ext
- **数据库**: PostgreSQL + pgvector

## 功能

1. 文档处理：上传 md/GDrive 文档，分块向量化存储
2. 知识库管理：products/modules 架构文档管理
3. 多 Agent 协同：DeepAgent 协调 4 个子 Agent（功能、运维、故障、边界测试）
4. 测试用例生成：根据需求+知识库生成 JSON 格式用例
5. 用例审核：用户审核修改后提交

## 输入约定

- 假定输入文档中的图片内容已在正文以文字描述覆盖；处理链路只对文字做清洗、分块、向量化与检索，不引入图片识别或 OCR。
- Markdown 中的 inline 与 reference-style base64 图片在清洗阶段会被丢弃，不进入分块与 embedding。

## 项目结构

```
CaseAgent/
├── backend/          # Go 后端
├── frontend/         # Vue 前端
├── docs/             # 规格、架构、回归证据、后续优化方向
├── scripts/          # 回归与冒烟脚本（用法见 scripts/README.md）
└── testdata/         # 公开 fixture
```

- 架构与回归索引：[`docs/architecture.md`](docs/architecture.md)
- 回归脚本说明：[`scripts/README.md`](scripts/README.md)
- 后续优化方向：[`docs/future_work.md`](docs/future_work.md)

## 快速开始

也可以直接使用仓库根目录的开发脚本同时启动前后端：

```bash
./dev.sh restart
```

默认地址：

- 前端：`http://localhost:40002/generate`
- 后端：`http://localhost:40003`

### 后端

```bash
cd backend
go mod tidy
# 本地私有配置优先读取 configs/.config.yaml；也可用 CASEAGENT_CONFIG 指定其他路径。
# cp configs/config-example.yaml configs/.config.yaml
go run cmd/server/main.go
```

启动时会自动按文件名顺序应用 `backend/migrations/*.sql` 中的当前 schema，测试库无需先手工建表。
后台 analyze / generate 由持久化 job runner 执行；并发、重试和 running job 超时恢复可在 `job_runner` 配置段调整。

> **多租户**：所有业务 API 都要求 `X-Tenant-ID` header（tenant slug）。`POST /api/v1/tenants` 创建租户后，请求带上 `-H "X-Tenant-ID: <slug>"` 即可。生产环境建议用 NOBYPASSRLS role 连接 DB（superuser 会绕过 RLS）—— 配置细节见 [`docs/multitenancy.md`](docs/multitenancy.md)。

### 前端

```bash
cd frontend
npm install
npm run dev
```

前端默认启动在 `http://localhost:40002`，首页会进入生成用例工作台。

## API 文档

- `POST /api/v1/tenants` - 创建租户（slug + name；不需要 X-Tenant-ID header）
- `GET /api/v1/tenants` - 列出租户（同上）

以下端点都要求 `X-Tenant-ID: <slug>` header：

- `POST /api/v1/projects` - 创建项目
- `GET /api/v1/projects` - 列出项目
- `GET /api/v1/projects/:id` - 获取项目详情
- `POST /api/v1/projects/:id/documents` - 上传文档
- `GET /api/v1/projects/:id/documents` - 列出文档
- `POST /api/v1/documents/:id/reprocess` - 重处理文档
- `POST /api/v1/knowledge` - 上传知识库
- `GET /api/v1/knowledge` - 列出知识库
- `POST /api/v1/knowledge/:id/reprocess` - 重处理知识库
- `POST /api/v1/projects/:id/tasks` - 创建生成任务
- `GET /api/v1/tasks/:id/cases` - 获取测试用例
- `GET /api/v1/knowledge-suggestions` - 列出知识建议
- `POST /api/v1/knowledge-suggestions` - 从用例手动反馈知识缺失
- `POST /api/v1/knowledge-suggestions/:id/draft` - 为知识建议生成待校对草稿
- `PUT /api/v1/knowledge-suggestions/:id` - 采纳/忽略知识建议，可回填 `resolved_knowledge_id`
- pending 知识建议会按 `suggestion.auto_dismiss_pending_days` 自动过期为 `dismissed`
- `GET /api/v1/maintenance/vector-health` - 查看向量健康状态
- `POST /api/v1/maintenance/reindex` - 批量重建异常向量

## License

This project is licensed under the [Apache-2.0 License](LICENSE-APACHE).
