# 依赖与架构基线刷新 - 2026-06-20

## 版本基线

- Go: 1.26.4（`go.mod` 使用 `go 1.26` 与 `toolchain go1.26.4`）
- Node/npm: Node 26.3.1、npm 11.17.0（`.node-version`、`packageManager` 与 `engines`）
- Eino: 0.9.9
- Eino 扩展: embedding/ark 0.1.2、embedding/openai `ab17b7308bf8`、model/ark 0.1.68、ACL OpenAI 0.1.17
- 后端核心依赖: Gin 1.12.0、Viper 1.21.0、Bun/pgdialect/pgdriver/bundebug 1.2.18
- 前端核心依赖: Vue 3.5.38、Vue Router 5.1.0、Pinia 3.0.4、Element Plus 2.14.2、Axios 1.18.0、Vite 8.0.16
- 前端构建工具: `@vitejs/plugin-vue` 6.0.7、`unplugin-auto-import` 21.0.0、`unplugin-vue-components` 32.1.0
- 仓库没有 Docker 基础镜像或 CI 配置，本轮无对应文件可刷新。

## 官方文档

- 已阅读与复核:
  - [Eino V0.9 更新注意事项](https://www.cloudwego.io/zh/docs/eino/release_notes_and_migration/eino_v0.9._agentic-runtime/eino_v0.9_migration_notes/)
  - [Go downloads](https://go.dev/dl/)
  - [Node.js releases](https://nodejs.org/en/about/previous-releases)
  - [Viper releases](https://github.com/spf13/viper/releases/tag/v1.21.0)
  - [Bun releases](https://github.com/uptrace/bun/releases/tag/v1.2.18)
  - [Gin changelog](https://github.com/gin-gonic/gin/blob/v1.12.0/CHANGELOG.md)
  - [Eino Extensions releases](https://github.com/cloudwego/eino-ext/releases)
  - [npm CLI releases](https://github.com/npm/cli/releases/tag/v11.17.0)
  - [Vue releases](https://github.com/vuejs/core/releases/tag/v3.5.38)
  - [Vue Router releases](https://github.com/vuejs/router/releases/tag/v5.1.0)
  - [Pinia releases](https://github.com/vuejs/pinia/releases/tag/v3.0.4)
  - [Element Plus releases](https://github.com/element-plus/element-plus/releases/tag/2.14.2)
  - [Axios changelog](https://github.com/axios/axios/blob/v1.18.0/CHANGELOG.md)
  - [Vite releases](https://github.com/vitejs/vite/releases/tag/v8.0.16)
  - [Vite Vue plugin releases](https://github.com/vitejs/vite-plugin-vue/releases/tag/plugin-vue%406.0.7)
  - [unplugin-auto-import releases](https://github.com/unplugin/unplugin-auto-import/releases/tag/v21.0.0)
  - [unplugin-vue-components releases](https://github.com/unplugin/unplugin-vue-components/releases/tag/v32.1.0)
- Eino 0.9 关键复核:
  - 当前应用不需要原生 Agentic 协议，继续使用官方兼容且未弃用的 `*schema.Message` 与 `model.BaseChatModel`。
  - 项目未使用已标记 NOT RECOMMENDED 的 Agent Transfer、Workflow Agent、Supervisor、Sequential/Parallel/Loop Agent。
  - 项目未使用本轮语义变化涉及的 ToolSearch middleware、ModelRetry、CancelError 或 AgenticModel tool binding。
  - 自定义 `adk.Agent`、`AgentInput`、`AgentEvent` 和 iterator 已由完整编译与后端测试验证；生成链路结果单独记录在下方 Demo 验证项。

## 架构变化

- 删除 Eino 已弃用的 `Message.MultiContent` 读取，只使用当前 `UserInputMultiContent` 与 `AssistantGenMultiContent`，并新增多模态 token 估算测试。
- HTTP handler 调用 Bun 与领域 service 时统一显式传递 `c.Request.Context()`，不再把 Gin context 作为标准 context 的兼容替代，tenant、取消信号和事务上下文沿标准边界传播。
- Bun debug query hook 改用 1.2 当前推荐的 `DB.WithQueryHook`，移除 deprecated `DB.AddQueryHook`。
- 空 project/knowledge 列表固定返回 `[]`，避免脚本和前端兼容 JSON `null`。
- `go mod tidy` 清除了未被代码使用的旧间接依赖，并按新直接依赖重新解析模块图。
- npm 锁文件已重新解析；Babel 8 RC 被正式版 8.0.0 替换，`form-data` 提升到 4.0.6，审计为 0 vulnerability。
- `demo_bootstrap.sh` 的 task 轮询进度改写到 stderr，命令替换只捕获最终状态，完整成功链路能稳定以 0 退出。

## 验证

- 后端依赖整理: `cd backend && go mod tidy` 通过。
- 后端静态检查: `cd backend && go vet ./...` 通过。
- 后端测试: `cd backend && go test ./...` 通过。
- PostgreSQL/RLS 集成测试: `CASEAGENT_TEST_DSN=postgres://caseagent_app:caseagent_app@127.0.0.1:5432/caseagent?sslmode=disable go test ./...` 通过。
- 前端安装: Node 26.3.1 + npm 11.17.0 下 `npm install` 通过，0 vulnerability。
- 前端构建: Node 26.3.1 + npm 11.17.0 下 `npm run build` 通过。
- 脚本检查: `bash -n scripts/*.sh scripts/lib/*.sh` 通过，`bash scripts/demo_bootstrap.sh --help` 通过。
- Demo fresh/生成链路: 固定 fake chat/embedding 下通过；run token `demo-20260620215647-20326`，project 80，document 20，task 70，最终状态 `completed`，生成 1 section / 1 case，trace 包含 6 agent runs / 5 model calls。
- 最终版本复查: Go 1.26.4、Eino 0.9.9、全部直接 Go/前端依赖均与本轮查询到的 2026-06-20 官方稳定版本一致；解析后的依赖树没有 alpha/beta/rc 包。

## 遗留风险

- Vite 8/Rolldown 构建会提示 `@vueuse/core@14.3.0` 中两处上游 `/* #__PURE__ */` 注释位置无法用于 tree-shaking；构建成功且不影响运行行为，等待上游稳定版修复，不在仓库中 patch 第三方产物。

## Git

- 实现 Commit: `4242322` (`chore: refresh dependency and architecture baseline`)
- Bun API 迁移 Commit: `14aac31` (`refactor: adopt current Bun query hook API`)
- Context 与 Demo 修复 Commit: `dcc008e` (`fix: propagate request context through handlers`)
- 回归记录 Commit: 本文件所在提交（见 git log）。
