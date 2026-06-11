# 后续优化方向（Future Work）

本文记录已识别但**尚未完成或尚未启动**的优化方向，按预期价值与成熟度排序。

当前团队假设：只有前端、后端工程师；没有算法工程师、数据工程师或专职测试工程师，也不计划补充这些角色。后续目标只纳入前后端工程师可以独立设计、实现、维护和验证的工作；需要模型训练、复杂数据平台、特征工程、专职测试平台或大规模离线实验的事项不进入当前计划。

## 团队能力边界

### 可以进入计划的工作

- 前端页面、交互、表格、筛选、批量操作、轻量审核流程。
- 后端 API、服务层、数据库模型、迁移、RLS / tenant 隔离、普通 SQL 聚合。
- 任务 trace、metadata、prompt/profile 版本记录、人工反馈样本沉淀。
- 本地脚本、固定样本报告、demo bootstrap/reset、README/脚本文档。
- 由前后端工程师维护的单元测试、集成测试、少量关键流程验证命令。

### 不进入当前计划的工作

- 需要算法工程师长期维护的模型能力：自动质量评分、语义重复检测模型、reranker、query rewrite 模型、embedding 训练、LLM-as-judge、大规模模型效果实验平台。
- 需要数据工程师长期维护的数据能力：数据仓库、BI 平台、特征工程 pipeline、复杂趋势预测、跨版本自动归因、大规模离线实验数据链路。
- 需要专职测试工程师长期维护的平台能力：完整自动化回归平台、大量端到端用例矩阵、跨浏览器/跨环境测试平台。

## 文档约定

每项使用三段式：

- **需要角色**：只写当前团队已有角色，例如前端工程师、后端工程师。
- **Trigger**：什么信号出现时启动这件事，避免过早优化。
- **DoD**：启动后怎么算完成，可机器复核；需要明确文件路径、API 路径、测试覆盖、验证命令。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护“做了哪些”。已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 前后端能做但当前不启动

这些事项不需要算法工程师或数据工程师，但不是当前最短路径。除非出现明确 Trigger，否则只保留为备选，不主动开工。

### API 契约与轻量 e2e

- **需要角色**：前端工程师、后端工程师。
- **Trigger**：API 调整频率下降，出现外部调用方，或前端接口字段回归开始频繁阻塞联调。
- **DoD**：维护 OpenAPI 或等价契约文件；前端 API client 由契约生成或至少有契约校验；新增 1-3 条 chromedp 浏览器主流程验证，覆盖 tenant 选择、任务创建、用例审核提交；`cd frontend && npm run build` 与后端相关测试通过。

### 任务生成前置检查

- **需要角色**：前端工程师、后端工程师。
- **Trigger**：任务失败或 demo 排障中反复出现文档未处理完成、知识库未完成向量化、tenant 未选、模型配置缺失、worker 未运行等可提前发现的问题。
- **DoD**：新增 `POST /api/v1/projects/:id/tasks/preflight` 或等价检查 API，返回 tenant、文档状态、知识库状态、模型配置、worker 风险的结构化 checklist；前端在创建任务和开始生成前展示阻塞项 / 警告项；补 handler/service 测试，`cd backend && go test ./internal/api/handler ./internal/service/...` 与 `cd frontend && npm run build` 通过。

### Demo 结果自检增强

- **需要角色**：前端工程师、后端工程师。
- **Trigger**：演示前仍需要人工打开 task/trace/cases 多个页面确认 demo 是否可用，或非工程同学使用 Demo 控制台后无法判断是否已经 ready。
- **DoD**：Demo 控制台展示 analyze/generate job 状态、task 终态、section/case 数、model_call 数和 trace 链接；失败时展示失败阶段与最近错误摘要；新增 API 或复用现有 `jobs` / `trace` / `cases` 接口，前端构建通过，并补至少一个 handler 或前端工具函数测试。

### API 错误码规范化

- **需要角色**：前端工程师、后端工程师。
- **Trigger**：前端需要根据错误类型做不同处理，或用户反馈中频繁出现无法区分 validation、conflict、not_found、retryable server error 的提示。
- **DoD**：后端主要 handler 的错误响应统一为 `{code,message,details}`，至少覆盖 tenant、task、testcase、knowledge、job 操作；前端 `frontend/src/api/client.js` 根据 `code` 归一化提示和 retryable 判断；补 handler 错误响应测试与前端构建验证。

### 运行时配置摘要页面

- **需要角色**：前端工程师、后端工程师。
- **Trigger**：排障或 demo 准备时频繁需要确认当前 chat/embedding provider、model、job runner、retention、index profile 等运行时配置。
- **DoD**：新增只读配置摘要 API，只返回非敏感字段并隐藏 secret；Ops 页面展示模型、向量、job runner、retention、indexing profile 摘要；补 handler 脱敏测试，确保 API 响应不包含 API key、access key、secret key、password 等字段。

### 本地验证入口收敛

- **需要角色**：前端工程师、后端工程师。
- **Trigger**：提交前需要手动记忆多条验证命令，或 review 中反复发现漏跑后端测试、前端构建、契约校验、demo/e2e smoke。
- **DoD**：新增 `scripts/check.sh` 聚合默认验证（`go test ./...`、`cd frontend && npm run build`、契约校验），并提供 `--e2e` 选项运行 chromedp happy path；`scripts/README.md` 记录入口、默认范围、耗时预期和失败排查方式。

## 暂不纳入计划

- 自动 OCR / 图片识别：README 已声明“输入文档中的图片内容已在正文以文字描述覆盖”。除非这个约定被破坏，否则不引入。
- 自动质量评分与算法优化：没有算法工程师前，不做自动覆盖率评分、LLM-as-judge、reranker、语义重复检测模型、query rewrite 模型、embedding 训练或模型效果实验平台。
- 数据平台与复杂分析：没有数据工程师前，不做数据仓库、BI 平台、特征工程 pipeline、复杂趋势预测、跨版本自动归因或大规模离线实验平台。
- 专职测试平台：没有专职测试工程师前，不做完整自动化回归平台、跨浏览器矩阵或大规模端到端用例平台；前后端工程师只维护与当前功能直接相关的测试。
