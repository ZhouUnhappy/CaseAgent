# 后续优化方向（Future Work）

本文记录已识别但**尚未启动**的优化方向，按预期价值与成熟度排序。

## 文档约定

每项使用两段式：

- **Trigger** —— 什么信号出现时启动这件事（避免过早优化）。
- **DoD** —— 启动后怎么算完成，可机器复核（参考 `git log` 中已 Done 任务的 DoD 格式：明确文件路径、API 路径、测试覆盖、验证命令）。

未列 Trigger 的项不要主动启动；列了 Trigger 的项在条件不满足前也不要启动。

不在本文维护"做了哪些"——已发生的事实由 git log 与 `docs/regression/` 承载。完成的项请直接从本文删除。

---

## P1：用户手动从用例发起 suggestion

**价值**：从"自动猜"到"有人参与"的质变。用户审核测试用例时最直观的判断是「这条用例提到了 X，但 X 在知识库里描述太单薄/根本没有」——这种判断只有人能下，且最准。

**Trigger**：第一批用户开始用产品并产生有意义的修改痕迹后。

**DoD**：

- 后端：`backend/internal/api/handler/knowledge_suggestion.go` 新增 `POST /api/v1/knowledge-suggestions`，请求体 `{candidate_type, candidate_name, source_case_id, source_task_id, note}`；写入时 `status=pending`，`source_snippets` 至少包含一条 `{type:"case", case_id, title}`；`candidate_type` 与现有枚举一致（`product` / `module`）。
- 前端：`frontend/src/views/TaskDetail.vue` 每条 case 行新增「反馈知识缺失」按钮，弹窗收集字段后调上述 API；成功后 toast 并刷新。
- 后端单测：handler 层新增 case 覆盖手动 POST 的字段校验（`case_id` / `task_id` 必填、`candidate_type` 枚举）。
- 验证：`scripts/i2_generation_e2e.sh` 跑通后，手工或新脚本验证一次 POST → 在 `GET /api/v1/knowledge-suggestions?status=pending` 中看到该条。

**关联**：和现有自动产生的 suggestion 共表，`status` / `candidate_type` 复用。

---

## P2：Adopted 与 knowledge_id 反向绑定

**价值**：suggestion adopted 后只是状态变了，没记录到底创建/更新了哪条 `knowledge_base.id`。后续这条 suggestion 复发时无法追溯第一次是谁采纳的、改了什么。

**Trigger**：发现"建议反复出现"，或审计场景需要回答"这条知识当时是因为哪个需求加的"。

**DoD**：

- Schema：`knowledge_update_suggestions` 新增 `resolved_knowledge_id` FK（migration `00X_*.sql`），指向同 tenant 下的 `knowledge_base.id`。
- 后端：`PUT /api/v1/knowledge-suggestions/:id` 在 `status` 改为 `adopted` 时接受可选的 `resolved_knowledge_id`，校验存在性和 tenant 归属。
- 前端：`KnowledgeBase.vue` 新建/编辑成功的回调里，如果 url 带了 `from_suggestion_id`，调一次 PUT 回填 `resolved_knowledge_id`；同时把 `KnowledgeSuggestions.vue` 跳转链接改为带 `from_suggestion_id`。
- 验证：跑一遍"建议 → 跳预填 → 保存知识 → 回到建议列表"，确认该建议状态为 `adopted` 且 `resolved_knowledge_id` 已填。

---

## P3：失败信号驱动的 suggestion

**价值**：当前 suggestion 只在 analyze 阶段产出。生成阶段的失败信号其实更值钱——子 Agent 全部失败、DeepAgent fallback 也失败、RefineCases 解析失败，都暗示「上下文不够」。

**Trigger**：生产环境累计出现 ≥3 次 task failed 且原因可能与上下文相关。

**DoD**：

- 业务类型：`knowledge_update_suggestions.candidate_type` 允许值新增 `context_gap`（当前 DB 列是 `VARCHAR(32)`；同步 handler/service/frontend 的类型判断，不引入数据库 enum）。
- 注入点：`backend/internal/service/agent/service.go` 的 fallback / parse 失败分支，调用 suggestion service 写入一条 `candidate_type='context_gap'` 的 suggestion，`source_snippets` 含当前 task 的 `affected_products` / `affected_modules` / 命中的 `knowledge_id` 列表 + 失败阶段标识。
- 前端：`frontend/src/views/KnowledgeSuggestions.vue` 列表能展示 `context_gap` 类型（与 `product` / `module` 区分颜色或图标），「采纳」对该类型禁用或改为"补充关联知识"流程。
- 单测：`backend/internal/service/agent/` 新增覆盖"agent 失败 → suggestion 写入"路径。
- 验证：手动模拟一次生成失败，确认表里出现 `candidate_type='context_gap'` 行。

---

## P4：跨任务聚合 + 优先级

**价值**：现在按 `(task_id, candidate_type, candidate_name)` 去重，同一个 `Billing-Core` 在 5 个 task 里都未被覆盖会产生 5 条独立 suggestion。审核体验差。

**Trigger**：suggestion 表行数 > 1000，或前端列表里出现「同一名字多副本」干扰审核。

**DoD**：

- Schema：新建聚合表 `knowledge_update_suggestion_groups`（主键按 tenant + `candidate_type + candidate_name`，含 `total_frequency` / `task_count` / `first_seen_at` / `last_seen_at`）和明细表 `knowledge_update_suggestion_occurrences`（记每次出现的 `source_task_id` / `source_snippets`）。现有 `knowledge_update_suggestions` 保留为兼容视图或过渡表，直到前端切换完成。
- 迁移：写 `002_*.sql`，把现有行按 `(tenant_id, candidate_type, candidate_name)` 聚合到 groups，detail 写入 occurrences。
- 服务层：`backend/internal/service/suggestion/service.go` 写入逻辑改为"先 upsert group，再 append occurrence"。
- 前端：`KnowledgeSuggestions.vue` 列表按 `task_count desc, total_frequency desc` 排序；点开某行展示 occurrences 子表。
- 验证：`scripts/i2_generation_e2e.sh` 多跑几次，确认主表行数稳定、子表行数累加。

---

## P5：中文实体识别

**价值**：现有 MVP 只识别英文标识符。需求里"对账核心"、"发票核对模块"这类中文复合词识别不出来——国内项目这类命名占大头。

**Trigger**：项目跑过几次发现 suggestion 表为空但需求里明显有未覆盖中文实体。

**DoD**：

- 选型：在 `backend/internal/service/suggestion/extractor.go` 接入轻量分词（候选：`gojieba` / `sego`），抽取名词性 token；或维护"白名单后缀"方案（`模块` / `服务` / `组件` 等）抓取「X模块、Y服务」复合词。
- 单测：`extractor_test.go` 新增 ≥3 个中文 case 覆盖名词抽取与停用词过滤。
- 验证：用一份含中文模块名的需求 fixture 跑 analyze，确认中文候选进入 suggestion 表。
- 不要破坏：现有英文识别测试必须仍通过。

---

## P6：采纳 → 自动生成知识条目骨架

**价值**：当前「采纳」只跳到知识库页预填 `type+name`，content 要用户从空白手写。如果有 30 条 pending suggestion 要逐条写，体验差。

**Trigger**：suggestion 数量上来（>20 条/周），且用户反馈"写起来累"。

**DoD**：

- 后端：新增 `POST /api/v1/knowledge-suggestions/:id/draft`，调用 chat model 把 `source_snippets` 喂给一个新 prompt，按知识库文档规范模板（概述 / 相关服务 / 工作原理…）生成 markdown 草稿；返回 `{draft_content}`。
- 前端：`KnowledgeSuggestions.vue` 「采纳」按钮先调上述 API（带 loading 态），把 `draft_content` 写入 `sessionStorage`（key 包含 suggestion id），再跳 `/knowledge?type=&name=&from_suggestion_id=<id>`；`KnowledgeBase.vue` onMounted 读取并清理该草稿。不要把完整 markdown 草稿塞进 URL query。
- Prompt 规范：在 `backend/internal/agent/` 下新增独立 agent 或在现有 agent 中加 prompt 文件；prompt 必须明确"草稿待人工校对"以避免 hallucination 直接落库。
- 单测：覆盖"无 source_snippets 时不调用 LLM、返回空 draft"的边界。

---

## P7：生命周期清理

**价值**：pending 状态会一直堆积，没人理就成了背景噪声。

**Trigger**：上线 1 个月后回头看 pending 是否在堆积（>200 条且最旧 >30 天）。

**DoD**：

- 后端：在 `backend/internal/service/suggestion/` 新增定时任务（启动时一次 + 每天一次），把 `created_at < now() - 30 days` 且 `status='pending'` 的行 update 为 `status='dismissed'`，附 `dismissed_reason='auto_expired'`（schema 加 `dismissed_reason` 字段）。
- 前端：列表过滤器默认隐藏 `dismissed_reason='auto_expired'`；保留切换显示选项。
- 配置：阈值（30 天）走 `backend/configs/config-example.yaml`，可环境变量覆盖。
- 验证：手工把某行 `created_at` 改老，重启后端，确认该行被自动 dismiss。

---

## 备忘：暂不做的事

- 自动 OCR / 图片识别：README 已声明"输入文档中的图片内容已在正文以文字描述覆盖"。除非这个约定被破坏，否则不引入。
- 多 chat provider（除 Ark 外）：模型决策已固定，要扩 provider 时另起方向。
