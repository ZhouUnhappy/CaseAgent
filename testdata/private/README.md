# 私有测试数据

此目录用于本地私有回归数据。仓库仅跟踪此目录下的 README、`.gitkeep` 和 `*.example` 文件，其他文件均被忽略。

使用方式：

```bash
export CASEAGENT_I1_PRIVATE_ARCH_DIR=/absolute/path/to/architectures
export CASEAGENT_I1_PRIVATE_INPUT_DIR=/absolute/path/to/inputs
export CASEAGENT_PSQL_DSN='postgres://...'
bash scripts/i1_private_corpus_eval.sh
```

不提交私有数据，仅在受版本管理的文档中保留清洗后的总结。
