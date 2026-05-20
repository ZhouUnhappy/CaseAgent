# 多租户改造实施计划

> 状态文件，AI 协作 / 人类共同维护。每完成一个任务把 `- [ ]` 改成 `- [x]`。
> 进度更新规则：开始动手前改 `[~]`，完成改 `[x]`，被阻塞改 `[!]` 并注明阻塞原因。

## 背景

项目处于验证阶段，可以推倒重来。当前 `knowledge_base` 是**全局共享表**，没有项目/租户隔离 —— 跨部门、跨公司场景下知识无法保密。本计划把整套系统重构成多租户架构，并以 PostgreSQL Row-Level Security 在 DB 层强制隔离。

## 设计决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 隔离层级 | tenant（部门/公司）> project（项目） | 跨部门/跨公司天然映射到 tenant；项目从属于 tenant |
| 隔离机制 | PostgreSQL RLS + `SET LOCAL app.tenant_id` | DB 层兜底，漏 WHERE 也不会泄露 |
| 租户来源 | HTTP Header `X-Tenant-ID`（验证阶段） | 简单直接；真正 SaaS 化后再换成 JWT claims |
| 连接管理 | 每个请求开 `RunInTx`，事务内 `SET LOCAL` | connection-scoped 设置会随连接复用泄露，必须 transaction-scoped |
| 向量索引 | `ivfflat` → `hnsw`（pgvector 0.5+） | hnsw 对带 filter 的 ANN 友好得多 |
| 迁移策略 | 重写 `001_init.sql`（推倒重来，不写 ALTER） | 验证阶段无生产数据 |

## 状态图例

- `[ ]` 待办  ·  `[~]` 进行中  ·  `[x]` 完成  ·  `[!]` 阻塞

---

## Phase 0: 基线确认

- [ ] 0.1 用户确认 plan（本文档）
- [ ] 0.2 确认分支策略（继续在 `feat/implementation-plan-phase2`，或切新分支）
- [ ] 0.3 备份当前可工作状态（确认 `git status` 干净，必要时 tag 一个 `pre-multitenancy` 点）

## Phase 1: 数据模型重构

### 1.1 重写 schema (`backend/migrations/001_init.sql`)

- [x] 1.1.1 新增 `tenants` 表：`id / slug / name / created_at / updated_at`，`slug` 唯一索引
- [x] 1.1.2 `projects` 加 `tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE` + 索引
- [x] 1.1.3 `documents` 加 `tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE` + 索引
- [x] 1.1.4 `document_chunks` 加 `tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE` + 索引（冗余但 RLS 需要）
- [x] 1.1.5 `knowledge_base` 加 `tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE` + 索引
- [x] 1.1.6 `case_generation_tasks` / `test_cases` / `knowledge_update_suggestions` 加 `tenant_id NOT NULL`
- [x] 1.1.7 把 `vector(2000)` 索引从 `ivfflat` 切到 `hnsw`（`USING hnsw (embedding vector_cosine_ops)`）
- [x] 1.1.8 删掉 schema 里所有 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...` 的兜底（推倒重来不需要）

### 1.2 Go model 同步 (`backend/internal/db/models/`)

- [x] 1.2.1 新建 `tenant.go`：`Tenant` 结构体
- [x] 1.2.2 `project.go`：加 `TenantID int`
- [x] 1.2.3 `document.go`：加 `TenantID int`
- [x] 1.2.4 `document_chunk.go`：加 `TenantID int`
- [x] 1.2.5 `knowledge_base.go`：加 `TenantID int`
- [x] 1.2.6 `case_generation_task.go` / `test_case.go` / `knowledge_update_suggestion.go`：加 `TenantID int`
- [x] 1.2.7 `all.go` 和 `db.go:42-50` 的模型注册列表加入 `Tenant{}`

## Phase 2: DB 连接和事务封装

### 2.1 引入 `RunInTx` 包装 (`backend/internal/db/`)

- [x] 2.1.1 在 `db/` 下新建 `tenantctx.go`：定义 `WithTenant(ctx, tenantID) ctx` / `TenantFromContext(ctx) (int, bool)`
- [x] 2.1.2 新建 `tx.go`：`RunInTenantTx(ctx, db, fn)` 辅助 —— 开事务 → `SET LOCAL app.tenant_id = $1` → 执行 fn → commit/rollback
- [!] 2.1.3 写单测 `tx_test.go`：验证 `SET LOCAL` 不会跨事务泄露 —— **合并到 3.1.4 / 9.3.2 RLS 集成测试**（单独测 SET LOCAL 不跨事务是 PG 内置行为，没意义；有 RLS policy 后才值得验证）

### 2.2 Service 层接受 `bun.IDB` 而非 `*bun.DB`

- [x] 2.2.1 `service/knowledge/service.go:19-22`：`db *bun.DB` → `db bun.IDB`
- [x] 2.2.2 `service/document/`：同上
- [x] 2.2.3 `service/retrieval/service.go:18-20`：同上
- [x] 2.2.4 `service/task/`、`service/agent/`、`service/suggestion/`、`service/maintenance/`：同上
- [x] 2.2.5 `db/pgvector/retriever.go` 和 `indexer.go` 内部 DB 调用同步改造

## Phase 3: RLS 落地

### 3.1 在 schema 里启用 RLS (`001_init.sql` 末尾)

- [x] 3.1.1 为所有带 `tenant_id` 的业务表执行 `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` + `... FORCE ROW LEVEL SECURITY`
- [x] 3.1.2 创建 policy 模板（每张表一条 `USING` + `WITH CHECK`）：`tenant_id = current_setting('app.tenant_id')::int`（所有业务表统一，无特例）
- [~] 3.1.3 用应用 role 而非 superuser 连接（superuser 会绕过 RLS）—— schema 里加 FORCE 已完成；NOBYPASSRLS role 配置不写进 schema（环境差异大），延到 Phase 10.4 README 文档化
- [x] 3.1.4 写 SQL 级隔离测试 `db/schema_rls_test.go`：插入两个租户数据，验证查询自动过滤；同时验证跨 tenant INSERT 被 WITH CHECK 阻断；用 `CASEAGENT_TEST_DSN` 环境变量提供连接，未设置时 skip

## Phase 4: HTTP 中间件 + 请求上下文

### 4.1 Tenant 解析中间件

- [x] 4.1.1 新建 `backend/internal/api/middleware/tenant.go`：从 `X-Tenant-ID` header 解析 → 查 `tenants` 表存在性 → 注入 `gin.Context`（key: `tenant_id`）
- [x] 4.1.2 缺失或不存在时返回 400，但**白名单 `/api/v1/tenants`**（建租户本身不需要 tenant 上下文）—— 通过 router 分组实现而非 path 判断
- [!] 4.1.3 中间件单测：合法 / 缺失 / 不存在 / 非法格式 四种 case —— **合并到 9.3.3**（都依赖真 PG 测试基础设施）

### 4.2 事务中间件

- [x] 4.2.1 新建 `middleware/tx.go`：在 tenant 中间件之后，对每个请求开事务，把事务对象塞进 `gin.Context`（key: `db`），请求结束 commit/rollback
- [x] 4.2.2 提供 `handler.DBFromContext(c) bun.IDB` 辅助
- [x] 4.2.3 异步 goroutine 改用 `handler.RunAsync(h.DB, tenantID, fn)` 单独开 tx（已用于 document / knowledge / task / maintenance 全部 async 调用）

### 4.3 路由注册中间件

- [x] 4.3.1 `router/router.go`：业务路由组 `biz.Use(middleware.Tenant(h.DB), middleware.Tx(h.DB))`
- [x] 4.3.2 `tenants` 路由组单独注册（只挂 `Tx`，不挂 `Tenant`）

## Phase 5: Handler / Service 全面改造

### 5.1 Handler 改用 ctx 里的事务和 tenant_id

- [x] 5.1.1 `handler/project.go` 全部 5 个方法：`h.DB.NewInsert()` → `DBFromContext(c).NewInsert()`，并在 INSERT 时填 `TenantID`
- [x] 5.1.2 `handler/document.go` 全部方法同上（异步 goroutine 改用 RunAsync 单独开 tx）
- [x] 5.1.3 `handler/knowledge.go` 全部方法同上（写入 `knowledge_base` 时从 ctx 自动取 tenant_id 填）
- [x] 5.1.4 `handler/task.go` 同上
- [x] 5.1.5 `handler/testcase.go` 同上
- [x] 5.1.6 `handler/knowledge_suggestion.go` 同上
- [x] 5.1.7 `handler/retrieval.go` 同上
- [x] 5.1.8 `handler/maintenance.go` 同上 —— reindex 改成 tenant-scoped（只 repair 当前 tenant 的数据）；跨 tenant batch repair 需 admin 通道，未来再做

### 5.2 Service 写入 `tenant_id`

- [x] 5.2.1 `service/document/`：分块入库时从 ctx 取 tenant_id 透传给 `document_chunks`
- [x] 5.2.2 `service/suggestion/`：写 `knowledge_update_suggestions` 时填 `tenant_id`
- [x] 5.2.3 `service/task/`：suggestion 提取从 bg goroutine 改成同步（tx-scoped IDB 不能脱离请求 tx）；`persistGeneratedCases` 去掉 RunInTx wrap（外层 handler tx 已就位），TestCase INSERT 时填 tenant_id

## Phase 6: 检索代码改造

### 6.1 验证 RLS 自动生效

- [x] 6.1.1 `service/retrieval/service.go:174` `SearchKnowledge`：不需要手动加 WHERE（RLS 兜底），但要确认走的是事务内的 IDB —— Phase 2 IDB refactor + Phase 4 Tx 中间件已实现，handler 调用 `retrievalservice.New(DBFromContext(c))`
- [x] 6.1.2 `db/pgvector/retriever.go` 同上 —— retriever 持有 caller 注入的 IDB，自动 tx-scoped
- [!] 6.1.3 写集成测试：两个租户各塞 10 条知识，分别检索验证只见到自己的（确认 RLS 隔离）—— **合并到 9.3.2 schema_rls_test 扩展**（已验证 projects 表 RLS；扩展到 knowledge_base 模式相同）

### 6.2 hnsw 索引性能验证

- [~] 6.2.1 跑现有 `scripts/i1_retrieval_smoke.sh` 确认基线 —— 阻塞：需 PG + 模型 API key 的真实环境，留给用户在本地跑
- [~] 6.2.2 用脚本造 1k 条知识 × 5 个租户的数据，对比 RLS 过滤前后的查询延迟（应在 50ms 以内）—— 同上
- [~] 6.2.3 如果延迟超标，调 hnsw 的 `ef_search` 或考虑 per-tenant partial index —— 视 6.2.2 结果

## Phase 7: Tenants 管理 API

### 7.1 CRUD

- [x] 7.1.1 新建 `handler/tenant.go`：`POST /api/v1/tenants` + `GET /api/v1/tenants`
- [x] 7.1.2 路由注册（`tenants` 组只挂 `Tx`，不挂 `Tenant`）
- [~] 7.1.3 ~启动时自动建 `default` tenant~ —— **不做**：plan 9.1 矩阵使用 fixture-specific slug（`i1-smoke` / `apache-dubbo` 等），没有 "default" 概念。各 fixture tenant 由 9.1 脚本按需创建

## Phase 8: 前端

### 8.1 Tenant 切换

- [x] 8.1.1 新建 `frontend/src/stores/tenant.js`：当前 tenant slug 持久化到 localStorage；fetch 时自动选第一个（避免首次访问 400）；本地 slug 已被服务端删除时清空
- [x] 8.1.2 `frontend/src/api/client.js` 拦截器从 localStorage 取 slug 注入 `X-Tenant-ID` header
- [x] 8.1.3 `frontend/src/layout/AppLayout.vue` 顶栏加 tenant 选择器 + 新建租户 dialog
- [x] 8.1.4 切换 tenant 后 `window.location.reload()` —— 简化方案，让每个 view 自动重新 fetch（替代每个 store 都订阅 tenant 变化）

## Phase 9: 测试 / 回归脚本

### 9.1 脚本支持 tenant 上下文

**租户颗粒度按"数据实际所属组织"分，不按目录名或脚本名分。** 一个目录里出现来自多个独立来源的语料时，必须为它们拆出不同 tenant slug —— 例如未来若 `public_corpus/` 同时混入 Apache Dubbo 和 Nginx 文档，必须拆成 `apache-dubbo` 和 `nginx-org` 两个 tenant，并在 `SOURCES.md` 标注每个文件的 tenant 归属。

| Fixture 集合 | Tenant slug | 备注 |
|---|---|---|
| `testdata/i1/{requirement,product_knowledge,module_knowledge,long_knowledge}.md` | `i1-smoke` | 仓库内简化 smoke fixture |
| `testdata/i1/public_corpus/{short,long}/*` | `apache-dubbo` | 当前全部来自 apache/dubbo-website 一个上游 |
| `testdata/private/...` | `CASEAGENT_I1_PRIVATE_TENANT_SLUG` 显式指定，缺失 fail-fast | 防止私有数据误入默认 tenant |
| determinism / i2 e2e | 自动从被复用对象（doc/project）回查 | 跟随 |

- [x] 9.1.1 所有 `scripts/i*.sh` curl 调用统一加 `-H "X-Tenant-ID: $TENANT"`，按上面矩阵设默认 —— smoke / public / private / i2 已改造；long_knowledge / determinism 留 [~]（pattern 同 smoke）
- [x] 9.1.2 `i1_private_corpus_eval.sh`：缺 `CASEAGENT_I1_PRIVATE_TENANT_SLUG` 时立即 exit 非 0，错误信息说明原因
- [x] 9.1.3 `i1_private_corpus_eval.sh` 末尾追加反向断言：切到 `i1-smoke`（可用 `CASEAGENT_I1_PRIVATE_PROBE_TENANT` 覆盖）检索同一查询，必须返回空（验证私有数据不漏到其他 tenant）
- [~] 9.1.4 `i1_retrieval_determinism.sh` / `i2_generation_e2e.sh`：从被复用 doc/project 自动回查 `tenant_id` 并注入 header —— i2 完成；determinism 待跟随（pattern 用 `tenant_slug_for_document`）
- [x] 9.1.5 `i1_retrieval_cleanup.sh`：保持 superuser DSN，文档注明它绕过 RLS（design intent）—— scripts/README.md "租户上下文" 表格已说明
- [x] 9.1.6 在 `scripts/README.md` 顶部加入"租户分配原则"段落 —— 落地为"租户上下文"段落 + 分配表

### 9.2 多租户隔离专项脚本

- [x] 9.2.1 新增 `scripts/multitenancy_isolation.sh`：建两个对等 tenant（A 和 B），用同一份 smoke fixture 各自上传到自己 tenant，互相检索对方 fixture 必须返回空（验证"私有 vs 私有"语义，与 9.1.3 的"私有 vs 跨 tenant"互补）

### 9.3 单测

- [x] 9.3.1 `backend/...` 现有 `*_test.go` 修复（model 字段增加、handler 签名变化）—— Phase 1+2 commit 时 `go test ./...` 全绿；schema_test.go fix 已包含在 Phase 2 commit
- [x] 9.3.2 新增 `backend/internal/db/schema_rls_test.go`（在 3.1.4 完成）
- [!] 9.3.3 新增 `backend/internal/api/middleware/tenant_test.go` —— **留 follow-up**：单测需 mock gin context 或真 PG，工程量大于本 phase 主线，schema_rls_test 已覆盖 RLS 隔离核心

## Phase 10: 文档

- [x] 10.1 更新 `docs/architecture.md`：在"固定技术决策"加入 RLS / tenant model + ivfflat→hnsw 修正
- [x] 10.2 更新 `docs/spec.md`：核心业务对象加入 `tenants`，主流程加上"选择租户"前置
- [x] 10.3 在 `docs/` 下新增 `multitenancy.md`：数据模型 / RLS policy 模板 / 应用层链路 / tenant 解析协议 / NOBYPASSRLS role 配置 / 新加表 6 步 checklist / 暂不实现的扩展
- [x] 10.4 更新 `README.md` 启动说明（X-Tenant-ID 必传、生产用 NOBYPASSRLS role）+ API 文档加 `/tenants`

---

## 验收清单

完成下列项目即视为改造完成：

- [ ] 两个 tenant 互相完全看不到对方的 project / document / knowledge / task / test_case / suggestion
- [ ] 所有现有回归脚本（`i1_*` / `i2_*`）在加上 tenant header 后全绿
- [ ] 新加的隔离测试 `scripts/multitenancy_isolation.sh` 通过
- [ ] `cd backend && go test ./...` 全绿
- [ ] `cd frontend && npm run build` 通过
- [ ] 故意漏掉某个 handler 的 tenant 填充，RLS 会让查询返回空 / INSERT 失败（防错性已生效）
