# Private Test Data

This directory is for local-only private regression data. The repository ignores all files here except this README, `.gitkeep`, and `*.example` files.

Recommended usage:

```bash
export CASEAGENT_I1_PRIVATE_ARCH_DIR=/absolute/path/to/architectures
export CASEAGENT_I1_PRIVATE_INPUT_DIR=/absolute/path/to/inputs
export CASEAGENT_PSQL_DSN='postgres://...'
bash scripts/i1_private_corpus_eval.sh
```

Do not commit private data. Keep only sanitized summaries in tracked docs.
