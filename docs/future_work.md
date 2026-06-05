# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 后续目标

### 1. 显式 Workflow 状态机

**Trigger** —— 后台任务开始出现跨 job 编排、人工审核回退、取消、重放、部分成功等状态，单靠 `background_jobs.status` 和 `workflow_runs.status` 已经无法表达真实业务进度。

**DoD** —— `backend/internal/service/workflow/` 提供集中状态转移 API，定义合法 transition、失败原因、取消语义和重放语义；job runner 和 task lifecycle 不再手写散落的状态更新；`backend/internal/service/workflow/*_test.go` 覆盖成功、失败、重试、取消、非法转移；`cd backend && go test ./...` 通过。

### 2. Trace 可靠性与关联完整性

**Trigger** —— 需要排查失败生成链路时，发现 agent/model/retrieval trace 会跟随业务事务回滚，或 `agent_runs` 与 `model_calls` 无法精确关联到同一次子 Agent 调用。

**DoD** —— trace 写入从业务事务中解耦，失败路径也能保留 retrieval / agent / model_call 记录；`model_calls.agent_run_id` 能关联到具体 `agent_runs.id`；`GET /api/v1/tasks/:id/trace` 返回完整关联视图；新增失败路径集成测试或 fake-provider 回归脚本证明失败 trace 可查询。

### 3. 原生 Agent Graph 编排

**Trigger** —— 子 Agent 之间需要共享中间产物、动态裁剪执行路径，或需要把 functional / ops / failure / boundary 真正作为可观测节点展示，而不是顺序函数调用。

**DoD** —— `backend/internal/service/agent/` 改为显式 agent graph 或 Eino ADK 编排；每个子 Agent 节点有独立输入、输出、错误和耗时 trace；DeepAgent fallback / refine 的触发条件在图中可见；现有 fake provider 测试和 `cd backend && go test ./...` 通过。

### 4. Demo Bootstrap 一键启动与种子数据

**Trigger** —— 需要给别人演示或录屏时，仍要手工创建 tenant、上传文档、上传知识、触发任务、等待生成。

**DoD** —— 新增 `scripts/demo_bootstrap.sh` 或等价命令，使用 fake provider 创建稳定 demo tenant/project/document/knowledge/task，并生成可展示的 completed task；脚本输出前端 URL、tenant slug、task id；`scripts/README.md` 记录前置条件和清理方式；本地可重复执行且不会污染私有语料 tenant。

### 5. Prompt Registry 与版本化生成策略

**Trigger** —— 为提升生成质量需要频繁调 prompt，或者需要对比不同 prompt/model 版本的质量和 trace。

**DoD** —— prompt 模板从各 Agent 代码中抽离到版本化 registry；生成时把 prompt version 写入 `model_calls.metadata` 或 workflow metadata；测试覆盖默认版本选择、缺失版本报错、fake provider 兼容；架构文档说明如何新增和回滚 prompt。

### 6. Retrieval / Index 版本化与重建队列

**Trigger** —— embedding provider、维度、chunk 策略或知识清洗规则变化后，需要确定哪些 document / knowledge 需要重建索引。

**DoD** —— document_chunks / knowledge_base 记录 index profile/version；维护 API 能列出 stale index，reindex 走 `background_jobs` 并写 workflow trace；`scripts/i1_*` 回归脚本能验证重建后检索稳定；RLS 测试覆盖新增表或字段。

### 7. 生成质量门禁与回归报告

**Trigger** —— 每次改 Agent、prompt、retrieval 或模型配置后，需要快速知道生成质量有没有退化。

**DoD** —— `scripts/i2_generation_quality_eval.sh` 输出结构化 JSON/Markdown 指标，至少覆盖 section 数、case 数、重复标题、字段完整率、source_context 覆盖率、失败阶段、model_call 次数和 token/字符统计；CI 或本地命令能用 fake provider 跑稳定门禁；`docs/regression/` 记录最近一次人工认可结果。

### 8. 运维工作台与人工干预能力

**Trigger** —— demo 之外开始需要长期运行 worker，或者需要在前端处理 stuck job、失败重试、取消、重放、查看跨 task workflow。

**DoD** —— 前端新增 job/workflow 运维视图，支持按 tenant、resource、status、job_type 筛选；后端提供必要的只读 API 和安全的 retry/cancel/replay API；所有干预动作写入 workflow trace；前端 build 和后端测试通过，文档说明哪些操作适合 demo、哪些需要生产权限模型。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
