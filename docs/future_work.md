# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## P3：失败信号驱动的 suggestion

**价值**：当前 suggestion 只在 analyze 阶段产出。生成阶段的失败信号其实更值钱——子 Agent 全部失败、DeepAgent fallback 也失败、RefineCases 解析失败，都暗示「上下文不够」。

**Trigger**：生产环境累计出现 ≥3 次 task failed 且原因可能与上下文相关。

**DoD**：

- 业务类型：`knowledge_update_suggestion_groups.candidate_type` 允许值新增 `context_gap`（当前 DB 列是 `VARCHAR(32)`；同步 handler/service/frontend 的类型判断，不引入数据库 enum）。
- 注入点：`backend/internal/service/agent/service.go` 的 fallback / parse 失败分支，调用 suggestion service 写入一条 `candidate_type='context_gap'` 的 suggestion，`source_snippets` 含当前 task 的 `affected_products` / `affected_modules` / 命中的 `knowledge_id` 列表 + 失败阶段标识。
- 前端：`frontend/src/views/KnowledgeSuggestions.vue` 列表能展示 `context_gap` 类型（与 `product` / `module` 区分颜色或图标），「采纳」对该类型禁用或改为"补充关联知识"流程。
- 单测：`backend/internal/service/agent/` 新增覆盖"agent 失败 → suggestion 写入"路径。
- 验证：手动模拟一次生成失败，确认表里出现 `candidate_type='context_gap'` 行。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
