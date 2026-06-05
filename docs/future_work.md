# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 统一后台任务体系

**Trigger** —— 文档处理、知识库处理、向量重建也需要和生成任务一样具备进程重启恢复、可配置重试、失败原因追踪，或 after-commit goroutine 的分散实现开始影响排查。

**DoD** —— 将非生成类后台 work 也迁入统一 job runner：

- 扩展现有 job 表或新增通用后台 job 表，能记录 job 类型、tenant、关联资源、payload、状态、重试次数、最后错误和时间戳。
- document process / reprocess、knowledge process / reprocess、maintenance reindex 的 handler 只提交 job。
- worker 统一 dispatch 各类 job，并使用 tenant-scoped tx，保留当前 RLS 隔离语义。
- 进程启动时能恢复 pending / running 超时 job；不同 job 类型支持独立并发和重试配置。
- 覆盖 enqueue、worker 执行、失败重试、进程重启恢复、tenant 隔离测试。
- `go test ./...` 通过；相关 smoke 脚本至少覆盖一次文档/知识库处理链路。

## 任务可观测性

**Trigger** —— 用户或开发者需要在前端/API 中定位后台任务卡住、失败或重试原因，而不是直接查数据库或日志。

**DoD** —— 暴露 job 状态并接入任务详情页：

- 新增 job 查询 API，支持按 task_id / document_id / knowledge_id / status 过滤，返回 job_type、status、retry_count、max_retries、last_error、run_after、started_at、finished_at。
- TaskDetail 展示 analyze / generate job 时间线，包含 pending / running / retrying / failed / succeeded 状态。
- 文档和知识库列表的失败行能展示最近一次 job 的错误摘要和重试次数。
- API 层继续遵守 tenant RLS；跨 tenant 查询不可见。
- 覆盖 handler 测试、job 查询 tenant 隔离测试；`npm run build` 通过。

## 生成质量评估

**Trigger** —— 生成逻辑、prompt、retrieval context 或模型配置发生变化，需要用固定样例判断输出质量是否退化。

**DoD** —— 增加可复跑的生成质量 eval：

- 在公开 fixture 上定义一组固定需求和期望覆盖点。
- 脚本记录模块/产品命中率、重复标题数、字段完整率、source_context 覆盖率、失败原因分布。
- 评估输出写入 `docs/regression/` 或 `testdata/i2/.../runs/`，可比较多次运行结果。
- eval 不要求 LLM 输出逐字一致，但必须有结构化阈值和失败原因。
- README / `scripts/README.md` 写清运行前置、环境变量和结果解释方式。

## LLM 假模型与确定性生成测试

**Trigger** —— 需要稳定覆盖真实 LLM 难以复现的正常/异常路径，例如格式错误、空结果、超时、限流、子 Agent 部分失败、job retry 和失败建议记录。

**DoD** —— 引入测试专用的 fake chat / embedding provider，不影响生产 provider：

- 抽象 chat / embedding provider 构造入口，使测试能注入 deterministic fake provider，生产仍按配置使用 Ark / DeepSeek / OpenAI-compatible。
- fake chat 支持按场景返回合法 JSON、非法 JSON、空数组、超时、限流/服务错误、子 Agent 部分失败。
- fake embedding 返回固定维度向量，维度由测试配置控制，避免依赖真实 embedding 服务。
- 增加不访问外部模型的生成链路测试，覆盖 analyze / generate 正常落库、parse failure、empty cases、timeout retry、context_gap 记录。
- 测试结果稳定可复跑；`go test ./...` 不需要真实 LLM API key 也能覆盖核心生成异常路径。

## 配置校验

**Trigger** —— 本地启动或回归脚本经常因为配置缺失、维度不一致、DB role 绕过 RLS、job_runner 参数不合理而运行到中途才失败。

**DoD** —— 启动阶段提前校验关键配置：

- 校验 chat / embedding provider 的必填字段、base_url、model、api_key / access_key / secret_key 组合。
- 校验 `model.embedding.dimensions` 和当前 pgvector schema 维度一致，不一致时给出明确错误或自动修正结果。
- 校验 runtime DB role 不是 superuser / BYPASSRLS，至少在非 debug 模式下拒绝启动。
- 校验 `job_runner` 的并发、重试、timeout、backoff 参数范围。
- 配置错误返回可读错误信息；覆盖 config 单测和启动路径测试。

## 前端任务体验

**Trigger** —— 生成任务排查、重试、等待过程主要依赖轮询状态文本，用户难以判断当前卡在哪个阶段。

**DoD** —— 工作台和任务详情页增加面向后台 job 的状态体验：

- CaseGenerationWorkspace / TaskDetail 展示任务阶段 timeline：created、analyze queued/running、awaiting_review、generate queued/running、retry、completed/failed。
- failed 状态展示最后错误摘要、失败阶段和可重试动作；retrying 状态展示下次运行时间。
- 列表页对 analyzing / generating / retrying 状态提供紧凑但可扫描的状态说明。
- 移动端和桌面端布局不重叠；`npm run build` 通过。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
