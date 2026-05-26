# 多租户架构

CaseAgent 的所有业务对象（projects / documents / document_chunks / knowledge_base / case_generation_tasks / test_cases / knowledge_update_suggestions）都强归属于一个 tenant。跨 tenant 数据互不可见，由 PostgreSQL Row-Level Security 在 DB 层兜底强制，应用层即使漏掉 WHERE 也不会泄露。

> 本文维护当前多租户架构约束；历史实施过程以 git log 为准。

## 数据模型

```
tenants
  ├─ id, slug (unique), name, created_at, updated_at

每张业务表都有
  ├─ tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  └─ btree index on tenant_id
```

`tenants` 表本身不启用 RLS —— 它是元数据，由 `/api/v1/tenants` 端点公开 list/create。其他 7 张业务表全部启用 `ENABLE ROW LEVEL SECURITY` + `FORCE ROW LEVEL SECURITY`。

## RLS Policy 模板

每张业务表挂同一条 policy（schema 文件 `001_init.sql` 末尾统一定义）：

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
COMMIT (or ROLLBACK)
```

- 关键 helper：
  - `db.WithTenant(ctx, tenantID)` / `db.TenantFromContext(ctx)` — context propagation
  - `db.RunInTenantTx(ctx, db, fn)` — 开 tx + 注入 SET LOCAL
  - `handler.DBFromContext(c) bun.IDB` — handler 拿到 tx-scoped 连接
  - `handler.TenantIDFromContext(c) (int, bool)` — handler 拿到 tenant id（写 INSERT 时填字段）
  - `handler.RunAsync(h.DB, tenantID, fn)` — 异步任务（请求结束后的 background 处理）必须用它单独开 tx，不能复用请求事务

- Caveat：gin 在 `c.JSON` 时已经把 response flush 给 client；我们的 tx commit 发生在 handler 返回之后。如果 commit 失败，client 已经看到 success status。日志会记录，但无法回滚 response。验证阶段够用；生产化要换成 buffered ResponseWriter。

## Tenant 解析协议

| 维度 | 选择 | 备注 |
|---|---|---|
| 传递方式 | HTTP Header `X-Tenant-ID` | 简单直接；SaaS 化后可换成 JWT claims |
| 值 | tenant slug（字符串） | 人类可读、URL 安全；中间件查表换成 int id |
| 缺失 / 不存在 | 400，`{"error":"missing X-Tenant-ID header"}` / `{"error":"tenant \"xxx\" not found"}` | 白名单路由 `/api/v1/tenants` 跳过此校验 |
| 前端 | localStorage(`caseagent.tenant_slug`) + axios request interceptor 自动注入 | 切换 tenant → `window.location.reload()` 让所有 view 重新拉数据 |

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
4. **handler**：所有 INSERT 路径填 `TenantID: handler.TenantIDFromContext(c)`；service.New 接 `bun.IDB`；异步任务用 `handler.RunAsync`
5. **测试**：扩展 `db/schema_rls_test.go`（或新建表的隔离测试）证明跨 tenant 不可见
6. **文档**：更新本文、`scripts/README.md` 和相关回归脚本说明，写清新表的 tenant 归属与验证方式

## 暂不实现的扩展

按 YAGNI 原则，下列特性当前**不做**，触发条件出现时再加：

- **`tenant_id IS NULL` 平台共享知识**：当前所有 knowledge 强归属 tenant。同一组件（Dubbo / Linux）在不同公司侧重点不同，"通用知识"塞共享池反而会稀释检索质量。
- **跨 tenant 的知识复制 / marketplace**：使用频率预计极低，UX 未经真实用户验证。真要做时走 `POST /api/v1/knowledge/import`（应用层端点临时切租户上下文读 + 写入当前 tenant），**不污染 RLS policy**。
- **跨 tenant 批量 reindex**：`POST /api/v1/maintenance/reindex` 当前只 reindex 当前 tenant 数据。真要批量跑需要独立 admin endpoint + superuser DSN，明确绕过 RLS。
- **完整 tenant CRUD**：当前只有 `POST` / `GET`。`DELETE` / `PUT` 留作运维需求出现时再加。
