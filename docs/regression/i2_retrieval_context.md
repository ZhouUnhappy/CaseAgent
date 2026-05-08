# I2 检索上下文回归（支撑 I2-T1）

记录"生成前的检索上下文增强 + 同 fixture 连续 3 次稳定性"的端到端样例。

## 样例 1：检索响应承载追溯字段（I2-T1）

### 字段约定

`POST /api/v1/retrieval/documents` 与 `POST /api/v1/retrieval/knowledge` 的 `items` 现在按以下结构返回（除既有字段外的新增）：

- 文档：`rank`（1-indexed 最终排名）、`best_score`（综合最高的 chunk 余弦相似度）、`hit_queries`（命中本文档的查询列表）；`matched_chunks[]` 由原 `[]string` 改为 `[]MatchedChunk{ text, score, query, rank }`，其中 `rank` 是该 chunk 在所属查询命中池中的位置，`score = 1 - cosine_distance`（pgvector `<=>` 算子）。
- 知识：`rank`、`score`、`hit_queries`。

`buildRequirementsContext`（`backend/internal/service/task/service.go`）在生成前把这些字段渲染成结构化 markdown 块（包含父文档 ID/名称、综合得分、每个命中片段的 score/query/chunk_rank），随后传给 `agent.GenerateCases`，确保 agent 上下文同时承载父文档身份、命中片段文本、检索 query 与片段排序+得分。

### 3 次连续执行稳定性

| 项 | 内容 |
| --- | --- |
| Fixture | `testdata/i1/public_corpus/{long,short}/`（与 I1-T7 同语料） |
| 文档查询 | `Dubbo 双注册原理 服务提供者 注册中心` / `模块发布器 服务发布全过程 ServiceConfig` / `Service Weaver 微服务编排 Google 论文` |
| 知识查询 | `Dubbo 流量管理 路由规则` / `Dubbo SPI 扩展点 加载机制` / `Dubbo 回调参数 异步通知` |
| 期望 | 同一查询连续 3 次返回的 `(document_id/id, name, rank, hit_queries, matched_chunks[].rank/query)` 完全一致；`score`/`best_score` 因嵌入 API 服务端浮点抖动可能在第 4 位小数后微变，不计入比较 |
| 执行命令 | `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_determinism.sh` |

### 最近一次实际结果摘要

最近一轮：本地基于 I1 公开语料 project_id=14（KEP 替代 Dubbo 长文 6 篇 + Dubbo 短文 15 篇）执行通过：

文档查询：

- ✓ `Dubbo 双注册原理 服务提供者 注册中心` → 3/3 identical，rank-1 document_id=29（best_score=0.7484838006761214）
- ✓ `模块发布器 服务发布全过程 ServiceConfig` → 3/3 identical，rank-1 document_id=27（best_score=0.672531419384465）
- ✓ `Service Weaver 微服务编排 Google 论文` → 3/3 identical，rank-1 document_id=28（best_score=0.7321092496600046）

知识查询：

- ✓ `Dubbo 流量管理 路由规则` → 3/3 identical，rank-1 knowledge_id=52（score=0.8254885250331316）
- ✓ `Dubbo SPI 扩展点 加载机制` → 3/3 identical，rank-1 knowledge_id=64（score=0.7547588501439271）
- ✓ `Dubbo 回调参数 异步通知` → 3/3 identical，rank-1 knowledge_id=53（score=0.7562281425023546）

详细本地报告：`.dev/i1_retrieval_determinism.md`（gitignored，重跑覆盖）。

### 关于 score 抖动

实测发现，`Qwen3-Embedding-8B` 通过 Gitee OpenAI-compatible 端点返回的向量在多次相同输入下会有 1e-4 量级的微抖动（疑似服务端 batched FP 累加顺序差异），导致 cosine score 在第 4 位小数后变化。**命中对象与排序不受影响**：本轮 3×6 = 18 次调用中 hit set + ordering 全部一致。

`scripts/i1_retrieval_determinism.sh` 的指纹（fingerprint）函数因此**只比对 hit set + ordering**，把浮点 score 排除出确定性判定。

## 复现执行流程

1. 准备前置环境（同 `i1_retrieval.md`）。
2. 执行 `bash scripts/i1_public_corpus_eval.sh` 把公开语料加载到 project（输出 run_token 与 project_id；脚本结束后 fixture 持久化在 DB 中）。
3. 执行 `CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_determinism.sh` 验证 3 次连续调用的 hit 一致性；脚本会自动从 DB 取最近一次 `I1 public corpus %` project 的 document_ids，无需手动传 ID。
4. 报告写入 `.dev/i1_retrieval_determinism.md`，把"3/3 identical"行回填到本文件「最近一次实际结果摘要」段。

## 历史失败与处置（可选）

记录有助于诊断的历史失败 + 处置；空段无需保留。
