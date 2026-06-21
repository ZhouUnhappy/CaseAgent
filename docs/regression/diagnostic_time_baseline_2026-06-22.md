# 诊断时间基线收敛回归记录

日期：2026-06-22

## 背景

任务、后台作业和 trace 链路曾混用无时区 `TIMESTAMP`、应用侧时间和数据库侧 `CURRENT_TIMESTAMP`，后续迁移又通过固定减去 8 小时修正旧数据。当前仍是可信本地 Demo，不迁移历史运行数据，直接以 UTC 和 `TIMESTAMPTZ` 作为新数据基线。

## 实现结果

1. `case_generation_tasks`、`background_jobs`、`workflow_runs`、`workflow_steps`、`agent_runs`、`model_calls`、`retrieval_runs` 和 `test_case_feedback` 的基础建表定义直接使用 `TIMESTAMPTZ`。
2. 删除 `011_diagnostic_timestamps.sql`，不再转换旧字段或按 6 至 10 小时时差窗口猜测并回写历史数据。
3. 新增统一 UTC 时钟入口 `internal/clock.Now()`，任务、作业、workflow、agent、model、retrieval、feedback 及诊断包的应用侧时间均从该入口取得；数据库连接继续通过 DSN 固定 `timezone=UTC`。
4. 后台任务进入成功或失败终态时，使用同一完成时刻写入 job 的 `finished_at` / `updated_at` 与对应任务的 `updated_at`；人工重试、取消和重放也统一使用 UTC 时钟，保证新任务时间线可稳定排序。
5. schema 测试逐表逐列检查 `TIMESTAMPTZ`，并拒绝固定时差及后置字段转换；PostgreSQL 集成测试验证 `task.created_at <= job.created_at <= job.started_at <= job.finished_at <= task.updated_at`，允许相邻时间相等。

## 自动化验证

- `cd backend && CASEAGENT_TEST_DSN='postgres://caseagent_app:caseagent_app@127.0.0.1:5432/caseagent?sslmode=disable' go test ./...`
- `cd frontend && npm run build`
- `git diff --check`

后端全量测试（含 PostgreSQL/RLS 集成测试）和前端生产构建通过。前端构建仍会输出 `@vueuse/core` 的 Rolldown `INVALID_ANNOTATION` 警告，该警告来自依赖且不阻断构建。

## Halo Demo 实跑

复用 `halo-thumbnail` tenant 中已导入的 Halo Issue #2387 与附件缩略图 RFC，不重建输入数据；新建任务 `111`，完整执行需求分析、影响范围审核和用例生成。

- 结果：任务 `completed`，生成 4 个类别、40 条测试用例。
- Job `31`（影响范围分析）：`17:22:27.797166Z <= 17:22:28.716504Z <= 17:22:30.850595Z`，状态 `succeeded`。
- Job `32`（用例生成）：`17:23:16.946730Z <= 17:23:18.711324Z <= 17:26:35.594836Z`，状态 `succeeded`。
- 任务 `updated_at` 为 `17:26:35.594836Z`，与生成 job 的完成时间一致。
- Trace：2 个 workflow runs、2 个 steps、6 个 agent runs、4 个成功 model calls、2 个 retrieval runs、40 条 case provenance。
- `deep_refine` 在 60 秒截止时间后记录 `context deadline exceeded`，系统按既有降级路径使用四个领域 Agent 的结果完成生成；该事件未造成 job 或任务失败。

## 桌面端核对

- 任务详情 `/tasks/111`：显示 4 类 40 条用例；技术诊断时间线依次展示任务创建、影响范围分析、用例生成、任务完成，时间为 `01:22:27 -> 01:22:28 -> 01:23:18 -> 01:26:35`。
- Ops `/ops` 运行记录：Job `31` 和 `32` 均显示已完成，开始时间分别为 `01:22:28`、`01:23:18`，耗时分别为 `2.1 s`、`197 s`。
- 常规桌面宽度下时间、状态、筛选和操作列无重叠；未进行窄屏或移动端验收。
