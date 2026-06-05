# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 后续目标

### 1. 任务详情与生成工作台 UX 收口

**Trigger** —— demo 用户开始主要通过前端排查任务，而不是看后端日志或直接查 trace API；任务详情页和生成工作台同时承载生成、审核、排障、编辑、提交后，页面信息密度让主流程不够清楚。

**DoD** —— `frontend/src/views/CaseGenerationWorkspace.vue` 与 `frontend/src/views/TaskDetail.vue` 明确区分主流程与诊断信息；默认视图突出任务选择、影响范围审核、生成结果和用例提交；job timeline、workflow trace、model call、retrieval、source_context 等排障信息收敛到诊断面板/抽屉/折叠区；失败状态提供清晰的下一步动作；`cd frontend && npm run build` 通过，并用本地页面或前端 smoke 验证 `/generate`、`/tasks/:id` 不白屏、关键按钮与状态可见。

### 2. 质量评估可视化

**Trigger** —— `scripts/i2_generation_quality_eval.sh` 已能输出质量指标，但 prompt / retrieval / model 调整后仍需要读 Markdown 或 JSON 才能比较生成质量，无法在前端快速看到质量趋势与失败分布。

**DoD** —— 新增质量报告查看入口（可为前端页面、静态 HTML 或后端只读 API + 页面），展示最近一次或指定 run 的 section/case 数、字段完整率、source_context 覆盖率、重复标题、失败阶段分布、model_call 数、字符/token 统计、prompt version 分布；`scripts/i2_generation_quality_eval.sh` 输出格式与可视化入口兼容；文档说明如何生成、查看和归档质量报告；`cd frontend && npm run build` 与相关脚本语法检查通过。

### 3. 真正的 AgentGraph / ADK 统一

**Trigger** —— 需要让 functional / ops / failure / boundary / DeepAgent 共享统一 Agent 接口、节点 metadata、动态裁剪执行路径，或需要把 DeepAgent coordination 从直接 chat 调用推进到可观测、可组合的 ADK 编排。

**DoD** —— 子 Agent 转成统一的 ADK/AgentGraph 节点接口，移除 `backend/internal/service/agent/service.go` 与 `backend/internal/agent/deep/deep.go` 中关于 sub-agent / coordination 的 TODO；graph 节点输入、输出、错误、耗时、prompt version、fallback/refine 触发原因都能写入 workflow trace；保留 partial failure 隔离、retry-once、DeepAgent fallback/refine 语义；`backend/internal/service/agent/*_test.go` 覆盖成功、单节点失败、全失败 fallback、refine 失败回退；`cd backend && go test ./...` 通过。

### 4. 请求事务与响应提交可靠性

**Trigger** —— 开始把服务用于长期运行或多人试用时，不能接受“客户端已看到成功，但事务 commit 失败只能写日志”的状态不一致；或者线上出现偶发提交失败、连接中断、RLS/DB 错误需要准确反馈给前端。

**DoD** —— `backend/internal/api/middleware/tx.go` 使用 buffered response writer 或等价机制，在事务提交成功前不向客户端 flush 成功响应；commit/rollback 失败能返回正确 5xx 并记录结构化日志；after-commit hook 只在真实 commit 成功后执行；新增 middleware 测试覆盖 handler 成功但 commit 失败、handler 4xx rollback、after-commit 执行时机；`cd backend && go test ./...` 通过。

### 5. 租户管理与无刷新切换

**Trigger** —— demo 从单人本地演示变成多人/多租户试用后，需要管理 tenant 生命周期、减少误切 tenant 造成的困惑，并避免通过 `window.location.reload()` 清空页面状态。

**DoD** —— 前端新增 tenant 管理视图或设置入口，支持查看、创建、重命名/归档（或明确删除策略）、设置默认 tenant；切换 tenant 后各 Pinia store 能无刷新重拉当前 tenant 数据，移除主要路径中的 `window.location.reload()`；后端补齐必要的 tenant update/archive API 与校验；文档说明 tenant 生命周期与 demo 清理方式；新增后端 handler 测试或前端 smoke 覆盖 tenant 切换；`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
