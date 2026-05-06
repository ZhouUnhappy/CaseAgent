# I1 检索回归样例

支撑 `IMPLEMENTATION_PLAN.md` 中 I1-T1 / I1-T2 / I1-T3 的回归证据。
每次 `bash scripts/i1_retrieval_smoke.sh` 通过后，在「最近一次实际结果摘要」追加最新一次 `run_token` 与命中详情。

## 前置环境

- 后端服务监听于 `CASEAGENT_BASE_URL`（默认 `http://localhost:8080/api/v1`），并已应用 `backend/migrations/001_init.sql`。
- PostgreSQL 已启用 `pgvector` 扩展，向量列维度与 `model.embedding.dimensions` 一致。
- `model.embedding` 配置可访问（Ark 或 OpenAI-compatible），后端启动后能成功调用 embedding 接口。
- 本地命令：`curl`、`jq`；启用 PSQL 校验时还需 `psql` 与 `CASEAGENT_PSQL_DSN`。

## 样例 1：文档检索（I1-T1）

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/requirement.md` |
| 查询词 | `probe-gate-7781 rollback completed` |
| 期望命中对象 | 本轮 smoke 上传的需求文档（rank 1） |
| 期望分块 | rank-1 条目至少有 1 条 `matched_chunks` |
| 执行命令 | `bash scripts/i1_retrieval_smoke.sh` |
| 启用数据库校验 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_smoke.sh` |

### 最近一次实际结果摘要

最近一轮：本地连续 3 次执行全部通过（CASEAGENT_I1_CLEANUP_LEGACY=1）。

- run 1：run_token=`i1-20260506180250-15437`，project_id=6，document_id=4，rank-1 document_id=4，document_chunks 行数=4 且 embedding 全非空。
- run 2：run_token=`i1-20260506180439-27429`，project_id=7，document_id=5，rank-1 document_id=5，document_chunks 行数=4 且 embedding 全非空（清理上一轮 2 行知识）。
- run 3：run_token=`i1-20260506180629-13861`，project_id=8，document_id=6，rank-1 document_id=6，document_chunks 行数=4 且 embedding 全非空（清理上一轮 2 行知识）。

最近一次完整日志：`/tmp/i1_smoke_run3.log`（脚本同时输出到 stdout）。

## 样例 2：知识库检索（I1-T2）

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/product_knowledge.md`、`testdata/i1/module_knowledge.md` |
| 查询词 | `control-plane-probe 18080 probe-gate-7781` |
| 期望命中对象 | 本轮 smoke 创建的 module 类型知识条目（rank 1） |
| 期望 metadata | rank-1 条目 `metadata.run_token` 等于本轮 RUN_TOKEN |
| 执行命令 | `bash scripts/i1_retrieval_smoke.sh` |
| 启用数据库校验 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_smoke.sh` |

### 最近一次实际结果摘要

最近一轮：本地连续 3 次执行全部通过（CASEAGENT_I1_CLEANUP_LEGACY=1）。

- run 1：run_token=`i1-20260506180250-15437`，product_knowledge_id=3，module_knowledge_id=4，rank-1 knowledge id=4，rank-1 metadata.run_token 与 RUN_TOKEN 一致；knowledge_base embeddings 非空 for {3,4}。
- run 2：run_token=`i1-20260506180439-27429`，product_knowledge_id=5，module_knowledge_id=6，rank-1 knowledge id=6，rank-1 metadata.run_token 一致；embeddings 非空 for {5,6}。
- run 3：run_token=`i1-20260506180629-13861`，product_knowledge_id=7，module_knowledge_id=8，rank-1 knowledge id=8，rank-1 metadata.run_token 一致；embeddings 非空 for {7,8}。

cleanup 在 run 2 / run 3 起跑前各删除上一轮 2 条 knowledge_base 行（按 metadata.aliases ⊇ ["I1 smoke fixture"]）。

## 复现执行流程

1. 准备前置环境（见上）。
2. （推荐）启用旧数据自动清理，避免上一轮 smoke 留下的同 alias 知识条目影响 rank-1 断言：
   ```
   export CASEAGENT_PSQL_DSN='postgres://...'
   export CASEAGENT_I1_CLEANUP_LEGACY=1
   ```
   也可以单独跑 `bash scripts/i1_retrieval_cleanup.sh --dry-run` 看候选。
3. 执行 `bash scripts/i1_retrieval_smoke.sh`，连续运行 3 次。每次结束前末尾会输出 `run_token=...`。
4. 任一次失败，按错误信息处理：
   - `document retrieval expected document <id> at rank 1` → 检查文档分块/embedding 是否成功；可启用 `CASEAGENT_PSQL_DSN` 让脚本同时校验数据库行。
   - `knowledge retrieval expected module knowledge <id> at rank 1` → 多由历史 smoke 数据干扰：`bash scripts/i1_retrieval_cleanup.sh` 清掉带 `metadata.aliases ⊇ ["I1 smoke fixture"]` 的旧行后重试。
5. 3 次都通过后，把最新一次的 `run_token`、`document_id`、`module_knowledge_id` 与 `assert_*` 输出回填到本文件「最近一次实际结果摘要」段落。

## 历史失败与处置（可选）

记录有助于诊断的历史失败 + 处置；空段无需保留。
