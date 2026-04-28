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

## 项目结构

```
CaseAgent/
├── backend/          # Go 后端
├── frontend/         # Vue 前端
├── docs/             # 规格说明等文档
└── IMPLEMENTATION_PLAN.md  # 实施计划
```

## 快速开始

### 后端

```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

启动时会自动应用 `backend/migrations/001_init.sql` 中的当前 schema，测试库无需先手工建表。

### 前端

```bash
cd frontend
npm install
npm run dev
```

## API 文档

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
- `GET /api/v1/maintenance/vector-health` - 查看向量健康状态
- `POST /api/v1/maintenance/reindex` - 批量重建异常向量

## License

This project is licensed under the [Apache-2.0 License](LICENSE-APACHE).
