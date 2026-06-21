# 当前优化项交付回归记录

日期：2026-06-22

## 实现结果

1. 诊断相关 task、job、workflow、step、agent、model、retrieval 和 feedback 时间列迁移为 `TIMESTAMPTZ`，数据库连接固定 UTC；前端页面统一通过 `frontend/src/utils/date.js` 转换本地时间。迁移包含受 RLS 保护的历史 job 八小时时差修复。
2. 类别提交改为以后端最新反馈为准：无反馈为待审核，`useful` 为通过，已隐藏的 `duplicate` 为已解决，其余反馈为未解决问题；待审核或未解决问题都会返回结构化 `409`。
3. 新增 `POST /api/v1/tasks/:id/cases/batch/feedback`，按 `test_case_id + case_index` 一次写入批量反馈；任务详情增加“批量通过”，批量标记重复也复用该接口。
4. 任务详情按 `tenant + task` 保存筛选、概览、高级筛选、类别/用例展开和聚焦位置；恢复时剔除失效类别与下标，并提供“重置视图”。
5. Ops 与租户表格的纯图标操作具有可访问名称，只显示当前状态可执行的 job 操作；用例步骤删除按钮也具有动态名称。
6. 全局头部只保留租户切换；租户创建收敛到租户管理页。生成完成态主操作改为“审核用例”，导出只保留在结果区，完成态不再显示简版诊断。
7. Ops 从按 API 划分的六个标签重组为运行概览、运行记录、质量与反馈、模型与稳定性、环境检查五个视图；Jobs/Workflows 与 Feedback/Quality 分别合并，视图按需加载，Preflight 默认展示可读清单并保留技术详情。

## 验证

- `cd backend && go test ./...`
- `CASEAGENT_TEST_DSN='postgres://caseagent_app:caseagent_app@127.0.0.1:5432/caseagent?sslmode=disable' go test ./...`
- `cd frontend && npm run build`
- `git diff --check`
- 启动后查询 `information_schema.columns`：30 个目标诊断时间列均为 `timestamp with time zone`。
- Task `#71` API 时间线：任务创建 `2026-06-20T16:33:33Z`，分析开始 `16:33:35Z`，生成开始 `16:34:21Z`，任务完成 `16:37:41Z`，顺序单调且均带时区。
- 常规桌面视口检查 Ops 五个视图、全局/租户创建入口和生成工作台；DOM 检查 Ops 可见按钮没有匿名项。

前端构建仍会输出 `@vueuse/core` 的 Rolldown `INVALID_ANNOTATION` 警告；该警告来自依赖且不阻断构建。

## 验收边界

浏览器已验证五个 Ops 视图的结构、中文主要操作、空状态、全局 tenant 上下文和无匿名按钮。带 PostgreSQL 的集成测试覆盖批量通过、重复提交、部分无效输入、tenant 隔离及类别问题收口；窄屏和移动端不在当前验收范围。
