# 后续优化方向（Future Work）

本文记录已识别但**尚未完成或尚未启动**的优化方向，按预期价值与成熟度排序。

当前团队假设：已有前端、后端、测试工程师；没有算法工程师、数据工程师，也不计划补充这些角色。后续目标只纳入前端、后端、测试工程师可以独立设计、实现、维护和验证的工作；需要模型训练、复杂数据平台、特征工程或大规模离线实验的事项不进入当前计划。

## 团队能力边界

### 可以进入计划的工作

- 前端页面、交互、表格、筛选、批量操作、轻量审核流程。
- 后端 API、服务层、数据库模型、迁移、RLS / tenant 隔离、普通 SQL 聚合。
- 测试工程师主导、前后端配合维护的接口自动化、脚本回归分级、固定样本冒烟验证。
- 任务 trace、metadata、prompt/profile 版本记录、人工反馈样本沉淀。
- 本地脚本、固定样本报告、demo bootstrap/reset、README/脚本文档。
- 由前后端工程师维护的单元测试、集成测试、少量关键流程验证命令。

### 不进入当前计划的工作

- 需要算法工程师长期维护的模型能力：自动质量评分、语义重复检测模型、reranker、query rewrite 模型、embedding 训练、LLM-as-judge、大规模模型效果实验平台。
- 需要数据工程师长期维护的数据能力：数据仓库、BI 平台、特征工程 pipeline、复杂趋势预测、跨版本自动归因、大规模离线实验数据链路。
- 重型测试平台能力：完整跨浏览器 UI 矩阵、海量端到端用例矩阵、商业测试平台建设。当前测试工程师优先做接口自动化和脚本回归，不把 Playwright 主流程作为默认路径。

## 文档约定

每项使用三段式：

- **需要角色**：只写当前团队已有角色，例如前端工程师、后端工程师。
- **Trigger**：什么信号出现时启动这件事，避免过早优化。
- **DoD**：启动后怎么算完成，可机器复核；需要明确文件路径、API 路径、测试覆盖、验证命令。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护“做了哪些”。已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

## 后续目标（当前建议启动）

### 1. 接口自动化回归

**需要角色**：测试工程师、后端工程师，前端工程师按需配合接口契约调整。

**Trigger**：核心链路已经覆盖 tenant / project / document / knowledge / analyze / review / generate / testcase / feedback / trace / demo fresh，后续任一改动都可能造成跨模块回归；测试工程师需要一套不依赖浏览器点击的稳定回归入口。

**DoD**：新增或整理接口自动化入口，建议路径为 `scripts/api_core_regression.sh` 或 `tests/api/`；覆盖创建/复用 tenant、创建 project、上传 document / knowledge、等待 analyze、提交 review、触发 generate、批量修改/提交用例、提交用例质量反馈、提交知识缺失反馈、读取 trace / source_context / generation profile；不新增 Playwright 主流程；失败时输出 API URL、tenant slug、project id、task id、最后 job error、trace last_error、model_call / retrieval_run 数量；在固定 fake provider demo 配置下可跑通；`bash -n scripts/*.sh`、`cd backend && go test ./...`、`cd frontend && npm run build` 通过。

### 2. 脚本回归分级与测试套件整理

**需要角色**：测试工程师、后端工程师。

**Trigger**：`scripts/i*.sh`、demo 脚本和质量评估脚本数量继续增加，人工记忆“什么时候跑哪个脚本”开始不可靠，或者 PR / 演示前需要明确的最小验证集合。

**DoD**：在 `scripts/README.md` 明确分级：smoke（快速接口/固定样本）、regression（检索/生成质量较慢验证）、demo（演示前 `demo_bootstrap.sh fresh`）；新增统一 runner 或 Make/NPM/脚本入口，建议 `scripts/run_regression_suite.sh`，支持选择 `smoke|regression|demo`；每个分级列出包含脚本、预计耗时、所需环境变量、是否需要真实模型；失败时保留可定位的日志和输出文件；不建设独立测试平台或可视化测试控制台。

### 3. 失败可诊断性标准化

**需要角色**：测试工程师、后端工程师。

**Trigger**：接口自动化或 demo fresh 失败后，需要靠手工查数据库、翻后端日志或多次重跑才能定位问题。

**DoD**：沉淀共享诊断 helper，建议路径为 `scripts/lib/diagnostics.sh`；脚本失败时统一输出 tenant slug、project id、document / knowledge id、task id、task status、最后一个 job error、trace last_error、agent / model / retrieval 计数、生成 profile id/version；接口自动化和 demo 脚本复用该 helper；不新增页面，不要求接入外部日志平台。

## 前后端能做但当前不启动

这些事项不需要算法工程师或数据工程师，但不是当前最短路径。除非出现明确 Trigger，否则只保留为备选，不主动开工。

- **OpenAPI / API 契约文档**：前后端和测试工程师可做。但当前先以接口自动化覆盖真实链路；只有外部调用方增加、API 变更开始影响多端协作，或测试用例需要稳定 schema 生成时，再补 OpenAPI / client 生成。
- **Playwright UI 主流程**：测试工程师可做。但当前页面仍是内部 demo / 管理界面，核心风险在 API、后台任务、数据落库与 trace；先不做 1-2 条 Playwright 主流程，避免维护浏览器点击流。
- **权限、操作者与审计**：前后端可做。当前仍按可信本地 demo / 小范围试用处理；只有出现外部用户、多人并发编辑、危险操作追责或客户侧安全要求时，再考虑登录、RBAC、操作者审计和危险操作二次确认。
- **Demo 控制台 / Demo 结果自检页面**：前后端可做。现阶段 demo 是工程内部使用，直接运行 `scripts/demo_bootstrap.sh fresh` 更便宜、可复现性更强；不继续扩展可视化一键 reset 或 demo 自检页面，演示前可用脚本输出确认 task id、case count、trace/model_call 计数。
- **知识库来源 / 状态筛选**：前后端可做。这里的“来源 / 状态”指 source/type/status 之类普通列表筛选，例如 manual/import/upload、product/module、processing/completed/failed；只有知识库数量明显增多、失败知识需要批量处理，或人工查找成本升高时再做。
- **知识引用影响查询**：前后端可做。可从测试用例 `source_context.knowledge_hits` 反查“哪些任务 / 用例引用过某条知识”；由于数据量可能变大，真正启动时必须按需查询、分页、限制时间窗口，必要时再加 JSONB index 或独立引用表。
- **知识更新影响提示**：前后端可做。实现方式是知识更新后按普通 SQL / JSONB 查询最近 N 天引用过该 knowledge id 的任务和用例，只提示“可能影响 X 个任务 / Y 条用例”，不自动重生成；只有知识频繁更新且用户开始追问影响范围时再做。
- **质量评估产品化页面**：前后端可做。prompt/profile 对比页、质量报告历史页、人工反馈汇总页可以从脚本报告演进而来；在固定样本报告尚未稳定前，不先做产品页。

## 暂不纳入计划

- 自动 OCR / 图片识别：README 已声明“输入文档中的图片内容已在正文以文字描述覆盖”。除非这个约定被破坏，否则不引入。
- 自动质量评分与算法优化：没有算法工程师前，不做自动覆盖率评分、LLM-as-judge、reranker、语义重复检测模型、query rewrite 模型、embedding 训练或模型效果实验平台。
- 数据平台与复杂分析：没有数据工程师前，不做数据仓库、BI 平台、特征工程 pipeline、复杂趋势预测、跨版本自动归因或大规模离线实验平台。
- 知识条目过期时间：当前没有明确业务需求，不做 expires_at、自动失效、过期提醒或过期清理。
- 重型测试平台：不做完整自动化回归平台、跨浏览器 UI 矩阵或大规模端到端用例平台；测试工程师优先维护接口自动化、脚本回归和少量关键验证命令。
