# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 异步任务运行器

**Trigger** —— 生成任务需要从本地 demo 进入多人 / 长时间运行场景，出现以下任一信号：进程重启后需要恢复未完成任务、需要限制并发生成数、需要查看后台任务失败原因、或需要对 analyze / generate 做可配置重试。

**DoD** —— 用持久化 job runner 替换 handler 直接 goroutine：

- 新增 job 表或复用任务表扩展，能记录 job 类型、tenant、task_id、状态、重试次数、最后错误和时间戳。
- handler 只提交 job；worker 统一领取并执行 analyze / generate。
- worker 使用 tenant-scoped tx，保留当前 RLS 隔离语义。
- 进程启动时能恢复 pending / running 超时任务。
- 覆盖并发上限、失败重试、进程重启恢复和 tenant 隔离测试。
- `go test ./...` 通过；可用本地模型和数据库时，`bash scripts/i2_generation_e2e.sh` 通过。

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
