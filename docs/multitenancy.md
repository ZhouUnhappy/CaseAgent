# 多租户架构

CaseAgent 的所有业务对象（projects / documents / document_chunks / knowledge_base / case_generation_tasks / background_jobs / workflow_runs / workflow_steps / agent_runs / model_calls / retrieval_runs / artifacts / test_cases / knowledge_update_suggestion_groups / knowledge_update_suggestion_occurrences）都强归属于一个 tenant。跨 tenant 数据互不可见，由 PostgreSQL Row-Level Security 在 DB 层兜底强制，应用层即使漏掉 WHERE 也不会泄露。

> 本文维护当前多租户架构约束；历史实施过程以 git log 为准。

## 数据模型

```
tenants
  ├─ id, slug (unique), name, archived_at, created_at, updated_at

每张业务表都有
  ├─ tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  └─ btree index on tenant_id
```

`tenants` 表本身不启用 RLS —— 它是元数据，由 `/api/v1/tenants` 端点公开 list/create/update/archive。归档 tenant 保留审计和历史数据，但默认列表隐藏；业务 API 的 tenant middleware 只解析 `archived_at IS NULL` 的 tenant。其他业务表全部启用 `ENABLE ROW LEVEL SECURITY` + `FORCE ROW LEVEL SECURITY`。

## RLS Policy 模板

每张业务表挂同一条 policy（schema 文件在建表迁移中定义）：

```sql
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS <table>_tenant_isolation ON <table>;
CREATE POLICY <table>_tenant_isolation ON <table>
    USING (tenant_id = current_setting('app.tenant_id')::int)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::int);
```

- `USING` 决定 SELECT / UPDATE / DELETE 看得到哪些行。
- `WITH CHECK` 决定 INSERT / UPDATE 能写入哪些行 —— 阻断跨 tenant 写入。
- `FORCE` 让表 owner 也受 policy 约束（默认 owner bypass）。
- `DROP POLICY IF EXISTS ... ; CREATE POLICY` 让 schema 可重复 apply。

## 应用层链路

```
HTTP Request
    │
    ▼ X-Tenant-ID: <slug>
middleware/tenant.go   ─► 查 tenants 表 → gin.Context.tenant_id = <id>
    │
    ▼
middleware/tx.go       ─► db.RunInTenantTx(ctx, db, fn)
                            ├─ BEGIN
                            ├─ SET LOCAL app.tenant_id = <id>
                            └─ fn(tx)  ← gin.Context.db = tx
    │
    ▼
handler.XxxFunc(c)     ─► DBFromContext(c).NewSelect/Insert/...
    │
    ▼ status >= 400 → tx rollback
COMMIT (or ROLLBACK) → flush buffered response
```

- 关键 helper：
  - `db.WithTenant(ctx, tenantID)` / `db.TenantFromContext(ctx)` — context propagation
  - `db.RunInTenantTx(ctx, db, fn)` — 开 tx + 注入 SET LOCAL
  - `handler.DBFromContext(c) bun.IDB` — handler 拿到 tx-scoped 连接
  - `handler.TenantIDFromContext(c) (int, bool)` — handler 拿到 tenant id（写 INSERT 时填字段）
  - `handler.RunAsyncAfterCommit(c, h.DB, tenantID, fn)` — 文档、知识库、维护类异步处理仍用它在请求提交后单独开 tx；不能复用请求事务。
  - 生成任务 analyze / generate 不走直接 goroutine；handler 只写 `background_jobs`，`service/job` worker 再用 tenant-scoped tx 领取并执行。
  - worker 会为每个 job 写入 `workflow_runs` / `workflow_steps`，业务服务继续在同一 tenant context 下记录 agent / retrieval / artifact trace。
  - `middleware/tx.go` 使用 buffered response writer，handler 成功响应会等事务 commit 和 after-commit hook 成功排程后再 flush；commit 失败时返回 5xx，避免客户端先看到成功。

## Tenant 解析协议

| 维度 | 选择 | 备注 |
|---|---|---|
| 传递方式 | HTTP Header `X-Tenant-ID` | 简单直接；SaaS 化后可换成 JWT claims |
| 值 | tenant slug（字符串） | 人类可读、URL 安全；中间件查表换成 int id |
| 缺失 / 不存在 / 已归档 | 400，`{"error":"missing X-Tenant-ID header"}` / `{"error":"tenant \"xxx\" not found"}` | 白名单路由 `/api/v1/tenants` 跳过此校验；归档 tenant 不再能访问业务 API |
| 前端 | localStorage(`caseagent.tenant_slug`) + axios request interceptor 自动注入 | 切换 tenant 会清空 tenant-scoped Pinia store 并用 RouterView key 触发当前页面重新拉数据，不做硬刷新 |

## 操作者与危险操作审计

当前仍是可信本地 demo / 小范围试用场景，不引入登录系统或 RBAC。需要追责的运维操作使用可信 header 注入操作者：

| Header | 含义 | 默认 |
|---|---|---|
| `X-Operator-ID` | 操作者稳定标识 | `local-demo` |
| `X-Operator-Name` | 操作者展示名 | 同 `X-Operator-ID` |

前端在 `localStorage(caseagent.operator_id)` / `localStorage(caseagent.operator_name)` 保存操作者，并在每个请求里自动注入。Jobs 的 `retry` / `cancel` / `replay` 和 `POST /api/v1/maintenance/reindex` 都要求 JSON body 带 `reason`，否则返回 400。后端会把 `operator_id` / `operator_name` / `reason` 写入 `artifacts.artifact_type='intervention'`：

- job 操作：`resource_type='job'`，`resource_id=<job_id>`，payload 含 action、job_id、job_type、status_before、operator 与 reason；`replay` 还会把 operator metadata 写入新 job payload。
- reindex 操作：`resource_type='maintenance'`，payload 含 action、operator、reason、index profile/version、queued/blocked document/knowledge 计数和 ID。

这不是安全边界。生产环境仍应把 operator header 换成登录态 / JWT claims，并在 API 前加 RBAC 与资源级授权。

## Tenant 生命周期与 demo 清理

- 创建：`POST /api/v1/tenants` 或前端顶部“新建”/`/tenants` 页面；slug 创建后不改名，显示名可通过 `PUT /api/v1/tenants/:slug` 更新。
- 默认 tenant：前端 `/tenants` 页面可把一个 active tenant 设为默认，保存在 localStorage(`caseagent.default_tenant_slug`)；当前 tenant 缺失或被归档时，前端优先选择默认 tenant。
- 归档：`POST /api/v1/tenants/:slug/archive` 设置 `archived_at`。归档不会删除业务数据，适合作为 demo / 试用环境的“停用”操作；业务 API 会拒绝这个 slug，普通租户下拉也不再显示。
- 恢复：`POST /api/v1/tenants/:slug/unarchive` 清空 `archived_at`，租户重新进入 active 列表。
- 清理：demo 数据优先按 project / knowledge / document 等业务 API 删除；需要保留历史但停止误用时归档 tenant。直接删除 tenant 会因业务表 `ON DELETE CASCADE` 清掉该 tenant 的业务数据，当前不暴露为前端操作，只建议在明确的一次性测试库清理中用 SQL 执行。

## NOBYPASSRLS Role 配置

PostgreSQL superuser **总是绕过 RLS**（这是 hardcoded 行为，`FORCE` 也无效）。生产部署和集成测试必须用非 superuser、不带 `BYPASSRLS` 属性的 role：

```sql
CREATE ROLE caseagent_app LOGIN PASSWORD '...' NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO caseagent_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO caseagent_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO caseagent_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO caseagent_app;
```

`backend/configs/config.yaml` 的 `database.user` / `database.password` 应填 `caseagent_app`，不要用初始化 schema 的 superuser。

集成测试 `db/schema_rls_test.go` 在开头检查 `pg_roles.rolsuper` 和 `rolbypassrls`，连接是 superuser / BYPASSRLS 时直接 `t.Skip`（policies 不会生效，测试无意义）。

`scripts/i1_retrieval_cleanup.sh` 是**唯一允许的例外**：它用 `CASEAGENT_PSQL_DSN`（一般指 superuser）直接 DELETE 历史 smoke 数据，绕过 RLS 是 design intent。

## 新加业务表 Checklist

每次给系统加新业务表（关联到 tenant 的数据），必须做这 6 件事，缺一就有泄露风险：

1. **schema**：列 `tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE` + btree index
2. **model**：Go struct 加 `TenantID int \`bun:"tenant_id,notnull" json:"tenant_id"\`` + 在 `db/models/all.go` 和 `db/db.go` 的 model 注册列表加入
3. **RLS**：schema 末尾追加 `ENABLE / FORCE ROW LEVEL SECURITY` + `DROP POLICY IF EXISTS` + `CREATE POLICY` 三行（复用同一模板）
4. **handler / worker**：所有 INSERT 路径填 `TenantID: handler.TenantIDFromContext(c)` 或 `db.TenantFromContext(ctx)`；service.New 接 `bun.IDB`；生成任务类后台 work 写入 `background_jobs`，worker 必须用 `db.RunInTenantTx`
5. **测试**：扩展 `db/schema_rls_test.go`（或新建表的隔离测试）证明跨 tenant 不可见
6. **文档**：更新本文、`scripts/README.md` 和相关回归脚本说明，写清新表的 tenant 归属与验证方式

## 暂不实现的扩展

按 YAGNI 原则，下列特性当前**不做**，触发条件出现时再加：

- **`tenant_id IS NULL` 平台共享知识**：当前所有 knowledge 强归属 tenant。同一组件（Dubbo / Linux）在不同公司侧重点不同，"通用知识"塞共享池反而会稀释检索质量。
- **跨 tenant 的知识复制 / marketplace**：使用频率预计极低，UX 未经真实用户验证。真要做时走 `POST /api/v1/knowledge/import`（应用层端点临时切租户上下文读 + 写入当前 tenant），**不污染 RLS policy**。
- **跨 tenant 批量 reindex**：`POST /api/v1/maintenance/reindex` 当前只 reindex 当前 tenant 数据。真要批量跑需要独立 admin endpoint + superuser DSN，明确绕过 RLS。
- **tenant 物理删除**：当前只暴露 create / list / update name / archive / unarchive。物理删除会级联删除业务数据，只留给明确的测试库清理脚本或人工 SQL。
