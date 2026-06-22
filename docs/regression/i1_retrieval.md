# I1 检索回归样例

支撑数据闭环（文档 / 知识库 → 向量化 → 检索）的回归证据。
每次 `bash scripts/i1_retrieval_smoke.sh` 通过后，在「最近一次实际结果摘要」追加最新一次 `run_token` 与命中详情。

## 前置环境

- 后端服务监听于 `CASEAGENT_BASE_URL`（默认 `http://localhost:40003/api/v1`），并已通过 `backend/schema/schema.sql` 初始化当前结构基线。
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

## 样例 3：长知识库整篇 embedding 召回评估（I1-T4）

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/long_knowledge.md` |
| 查询词 A | `ingest-relay-3209 19090 数据集成` |
| 查询词 B | `notify-center-9921 21100 通知` |
| 查询词 C | `billing-pulse-4477 23330 计费` |
| 期望命中对象 | 本轮创建的长知识库条目在每个查询的 `top_k=5` 内命中 |
| 执行命令 | `bash scripts/i1_long_knowledge_eval.sh` |
| 启用数据库校验 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_long_knowledge_eval.sh` |

### 最近一次实际结果摘要

最近一轮：本地执行 `bash scripts/i1_long_knowledge_eval.sh` 通过。

- run_token=`i1-long-20260507161825-10193`，long_knowledge_id=9，知识条目状态最终为 `completed`。
- 查询 A：`ingest-relay-3209 19090 数据集成`，rank-1 knowledge id=9。
- 查询 B：`notify-center-9921 21100 通知`，rank-1 knowledge id=9。
- 查询 C：`billing-pulse-4477 23330 计费`，rank-1 knowledge id=9。
- 本轮未设置 `CASEAGENT_PSQL_DSN`，脚本跳过数据库 embedding 非空直接校验；检索成功已验证 embedding 可用于召回。

## 样例 4：私有真实语料回归（I1-T6）

| 项 | 内容 |
| --- | --- |
| 数据来源 | 本地私有架构知识目录 + 本地私有需求/设计输入目录 |
| 执行命令 | `CASEAGENT_I1_PRIVATE_ARCH_DIR=... CASEAGENT_I1_PRIVATE_INPUT_DIR=... CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_private_corpus_eval.sh` |
| 隐私约束 | 私有数据目录与详细报告均位于 gitignored `testdata/private/`，仓库只记录脱敏统计摘要 |
| 期望结果 | 文档与知识库均完成处理；chunk 与 embedding 数量 > 0；典型文档/知识查询在 `top_k=5` 内命中本轮对象 |

### 最近一次实际结果摘要

最近一轮：本地执行 `scripts/i1_private_corpus_eval.sh` 通过。

- run_token=`i1-private-20260507170014-5662`，project_id=12。
- 架构知识：8 个 md，raw bytes=18,780，cleaned knowledge bytes=18,788，knowledge_embeddings=8。
- 需求/设计输入：5 个 md，raw bytes=4,204,164，cleaned document bytes=85,943。
- 文档处理：document_chunks=101，document_embeddings=101。
- 文档检索：`VDS IGMP Snooping 组播`、`mcast-agent query 报文`、`全局默认拒绝 组播` 均 rank-1 命中本轮上传文档。
- 知识检索：`VDS IGMP MLD Snooping`、`OVS Bridge OpenFlow`、`Everoute 分布式防火墙` 均 rank-1 命中本轮知识条目。
- 详细本地报告：`testdata/private/runs/i1-private-20260507170014-5662.md`（已被 `.gitignore` 忽略，不提交）。

## 样例 5：公开复杂文档语料回归（I1-T7）

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/public_corpus/long/`（6 篇 Apache Dubbo 中文长文）+ `testdata/i1/public_corpus/short/`（15 篇 Apache Dubbo 中文短文） |
| 上游 | `apache/dubbo-website` @ `19e75d7c79c2a7fcf13477d47aac8a0867ea704c`（Apache-2.0），抓取细节见各子目录 `SOURCES.md` |
| 文档查询 | `Dubbo 双注册原理 服务提供者 注册中心` / `模块发布器 服务发布全过程 ServiceConfig` / `Service Weaver 微服务编排 Google 论文` |
| 知识查询 | `Dubbo 流量管理 路由规则` / `Dubbo SPI 扩展点 加载机制` / `Dubbo 回调参数 异步通知` |
| 期望命中对象 | 长文查询在 `top_k=5` 内 rank-1 命中本轮上传对象；短文知识查询在 `top_k=5` 内命中本轮上传对象 |
| 执行命令 | `CASEAGENT_PSQL_DSN='postgres://...' CASEAGENT_I1_CLEANUP_LEGACY=1 bash scripts/i1_public_corpus_eval.sh` |

### 最近一次实际结果摘要

最近一轮：本地执行 `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_public_corpus_eval.sh` 通过。

- run_token=`i1-public-20260531134530-8970`，project_id=54。
- 长文：6 个 md，raw bytes=235,944，cleaned document bytes=235,950，document_chunks=252，document_embeddings=252。
- 短文：15 个 md，raw bytes=74,733，cleaned knowledge bytes=74,748，knowledge_embeddings=15。
- 文档检索 rank-1 命中：document_id=12 (`dubbo-provider-dual-register.md`)、10 (`dubbo-module-publisher.md`)、11 (`dubbo-service-weaver-paper.md`)。
- 知识检索命中本轮上传对象：knowledge_id=38 (`concepts-traffic-management.md`, rank-2)、50 (`concepts-extensibility.md`, rank-1)、39 (`advanced-callback-parameter.md`, rank-2)。
- 详细本地报告：`testdata/i1/public_corpus/runs/i1-public-20260531134530-8970.md`（已被 `.gitignore` 忽略，不提交）。

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
6. 执行 `bash scripts/i1_long_knowledge_eval.sh` 评估长知识库整篇 embedding；如历史长知识库 fixture 过多，可配合 `CASEAGENT_I1_CLEANUP_LEGACY=1` 与 `CASEAGENT_PSQL_DSN` 清理 `metadata.aliases ⊇ ["I1 long knowledge fixture"]` 的旧行后重试。
7. 执行 `bash scripts/i1_private_corpus_eval.sh` 评估本地私有真实语料；私有目录路径通过环境变量传入，详细报告写入 gitignored `testdata/private/runs/`。
8. 执行 `bash scripts/i1_public_corpus_eval.sh` 评估仓库内公开复杂语料（`testdata/i1/public_corpus/`）；如历史 fixture 过多，可配合 `CASEAGENT_I1_CLEANUP_LEGACY=1` 与 `CASEAGENT_PSQL_DSN` 清理 `metadata.aliases ⊇ ["I1 public corpus fixture"]` 的旧行后重试，详细报告写入 gitignored `testdata/i1/public_corpus/runs/`。

## 历史失败与处置（可选）

记录有助于诊断的历史失败 + 处置；空段无需保留。
