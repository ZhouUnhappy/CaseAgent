# I1 Retrieval Fixtures

这些 fixture 用于 `Iteration 1` 的真实联调：

- `requirement.md`：需求文档上传、分块、embedding 和文档检索样例。
- `product_knowledge.md`：产品知识库样例。
- `module_knowledge.md`：模块知识库样例，包含长知识库检索评估会复用的唯一查询词。
- `long_knowledge.md`：I1-T4 长知识库整篇 embedding 召回评估样例，包含 3 个相互独立主题。

运行方式：

```bash
bash scripts/i1_retrieval_smoke.sh
```

评估长知识库整篇 embedding 召回：

```bash
bash scripts/i1_long_knowledge_eval.sh
```

默认后端地址为 `http://localhost:8080/api/v1`。如需覆盖：

```bash
CASEAGENT_BASE_URL=http://localhost:8081/api/v1 bash scripts/i1_retrieval_smoke.sh
```

如果需要同时验证 `document_chunks` 行数，可提供 PostgreSQL DSN：

```bash
CASEAGENT_PSQL_DSN='postgres://user:pass@localhost:5432/caseagent?sslmode=disable' bash scripts/i1_retrieval_smoke.sh
```
