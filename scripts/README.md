# scripts/

CaseAgent 的回归脚本集合。每个脚本都是**独立的回归工具**，不构成顺序流水线——按需挑用即可，互相不依赖（唯一例外是 `i1_retrieval_cleanup.sh` 是 smoke 的反操作）。

## 租户上下文

所有 `i*.sh` 现在要求请求带 `X-Tenant-ID` header（多租户改造后强制；参见 `docs/multitenancy.md`）。每个脚本顶部用 `TENANT_SLUG` 变量声明默认值：

| 脚本 | 默认 TENANT_SLUG | 备注 |
|---|---|---|
| `i1_retrieval_smoke.sh` | `i1-smoke` | 仓库内简化 fixture |
| `i1_public_corpus_eval.sh` | `apache-dubbo` | 全部 fixture 来自 apache/dubbo-website |
| `i1_private_corpus_eval.sh` | `CASEAGENT_I1_PRIVATE_TENANT_SLUG` 必填，不给直接 exit | 防止私有数据误入默认 tenant |
| `i2_generation_e2e.sh` | `apache-dubbo` | 默认复用最新 public-corpus project；复用其他 project 时同时设置 `CASEAGENT_TENANT_SLUG` |
| `multitenancy_isolation.sh` | 一次性创建 `iso-a-*` / `iso-b-*` 两个临时 tenant | 验证私有 vs 私有隔离 |
| `i1_retrieval_cleanup.sh` | **绕过** RLS（用 `CASEAGENT_PSQL_DSN` superuser 直接 DELETE） | design intent |

**`scripts/lib/tenant.sh`** 提供共享 helper：`ensure_tenant`（若不存在则 POST 创建）、`psql_tenant`（为 app role 的直连 SQL 注入 `SET LOCAL app.tenant_id`）、`tenant_slug_for_document` / `tenant_slug_for_project`（从 doc/project id 回查 slug，供少量兼容路径使用）。

**新增脚本约定**：必须在文档（README + 脚本头注释）里说明默认 `TENANT_SLUG` 及理由；如果脚本面向多个独立来源的语料，必须为每份语料声明独立的 tenant slug，不能塞进同一个 tenant。

## 通用前置

- 后端起在 `http://localhost:8080`（或用 `CASEAGENT_BASE_URL` 覆盖）。
- PostgreSQL + pgvector 已启动，schema 已迁移（后端启动时会自动 apply `backend/migrations/001_init.sql`）。
- 模型配置（chat / embedding）有效，能真发请求。
- `curl` / `jq` / `psql` 可用。

很多脚本读 `CASEAGENT_PSQL_DSN` 直连数据库做断言或选数据：

```bash
export CASEAGENT_PSQL_DSN='postgres://user:pass@localhost:5432/caseagent?sslmode=disable'
```

## 数据闭环（i1_*）

### `i1_retrieval_smoke.sh`

**用途**：最小冒烟。上传 1 个需求文档 + 2 条知识库 → 等处理完成 → 同一查询 top1 必须命中本轮对象 → 连跑 3 次。

**输入**：默认读 `testdata/i1/{requirement,product_knowledge,module_knowledge}.md`，可用 `CASEAGENT_I1_*_FIXTURE` 覆盖。

**输出**：stdout 进度日志 + 失败时 exit 非 0。

**何时跑**：改了文档/知识库 ingest 路径、改了 retrieval、升级 embedding 模型时。

---

### `i1_long_knowledge_eval.sh`

**用途**：验证长知识库分块/聚合策略——一篇含 3 个独立主题的长 fixture，3 个不同查询各自 top_k=5 内命中对应主题段。

**输入**：`testdata/i1/long_knowledge.md`（可 `CASEAGENT_I1_LONG_KNOWLEDGE_FIXTURE` 覆盖）。

**何时跑**：改了 chunker、改了知识库 embedding 聚合逻辑时。

---

### `i1_retrieval_determinism.sh`

**用途**：同 fixture / 同查询连跑 3 次，断言命中集合 + 排序完全一致。

**输入**：默认从最近一次 public-corpus run 的文档 ID 列表里选；如未设置，需要 `CASEAGENT_I1_DETERMINISM_DOC_IDS` 显式给。

**输出**：默认 `.dev/i1_retrieval_determinism.md`。

**何时跑**：怀疑检索结果有非确定性抖动（embedding cache、向量索引、随机化排序）时。

---

### `i1_private_corpus_eval.sh`

**用途**：用本地真实语料完整跑链路，记录"文件数 / 字节数 / 清洗后字节数 / chunk 数 / embedding 成功数 / 典型查询命中"。

**输入**（必需）：

- `CASEAGENT_I1_PRIVATE_ARCH_DIR` —— 真实架构知识目录
- `CASEAGENT_I1_PRIVATE_INPUT_DIR` —— 真实需求/设计输入目录

**输出**：`testdata/private/runs/<run_token>.md`（私有目录，已 `.gitignore`）。

**何时跑**：评估真实业务语料的清洗/分块/检索效果；私有数据不进 git。

---

### `i1_public_corpus_eval.sh`

**用途**：同上，但读仓库内的公开 fixture，结果可提交。

**输入**：默认 `testdata/i1/public_corpus/{long,short}/`，分别是长篇设计文档和短篇架构知识。

**输出**：`testdata/i1/public_corpus/runs/<run_token>.md`。

**何时跑**：希望产生**可分享、可复跑**的回归证据；CI 也应使用此脚本。

---

### `i1_retrieval_cleanup.sh`

**用途**：删历史 smoke 残留的 knowledge_base 行（`metadata.aliases ⊇ ["I1 smoke fixture"]`），避免 smoke 排名断言被旧数据污染。

**输入**：必需 `CASEAGENT_PSQL_DSN`；支持 `--dry-run`。

**何时跑**：smoke 出现"top1 命中的不是本轮新增对象"时。先 `--dry-run` 看候选再真删。

---

## 生成闭环（i2_*）

### `i2_generation_e2e.sh`

**用途**：在已有的 public-corpus 项目上走完 analyze → review → generate 全流程，并断言：

- `duplicate_title_count == 0`（去重生效）
- `cases_missing_affected_fields == 0`（每条用例都带受影响产品/模块）
- `sections_with_source_context == section_count`（每个 section 都有可追溯的源上下文）

**输入**：

- 必需 `CASEAGENT_PSQL_DSN`（用来定位项目、校验落库）。
- 可选 `CASEAGENT_I2_E2E_PROJECT_ID` / `CASEAGENT_I2_E2E_DOCUMENT_ID`，不给则自动选最近一个 `I1 public corpus%` 项目和最新 completed 文档。

**输出**：默认 `.dev/i2_generation_e2e.md`。

**何时跑**：改了 Agent 编排、解析、去重、source_context 拼装、或任何会影响生成结构的逻辑时。

---

## 何时该新增脚本

新功能的回归如果满足以下任一条，建议加脚本而不是只跑一次：

1. 涉及多个组件（API + 后台任务 + DB）的 e2e 路径。
2. 有数据相关的非确定性风险（向量、模型输出、并发）。
3. 改某模块时容易回归别的模块。

新增脚本的命名约定：`<闭环编号>_<功能>.sh`（i1 = 数据闭环，i2 = 生成闭环，依此类推）。在本 README 表格中补一行，并在 `docs/architecture.md` 对应小节加引用。
