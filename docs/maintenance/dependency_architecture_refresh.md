# 依赖与架构基线刷新 Playbook

本文是可重复执行的长期维护 Playbook，不是一次性待办。每轮执行仅负责将项目刷新到当时的最新稳定基线；满足停止条件后必须结束当前轮次，等待下一次 Trigger。

## 目标

将项目刷新到当前最新稳定技术基线，并采用各依赖官方当前推荐的 API、写法和架构。判断依据以官方 release notes、migration notes 和最新稳定文档为准，不以保留现有实现为目标。

## 必须阅读的官方资料

- Eino V0.9 更新注意事项：[CloudWeGo 官方迁移文档](https://www.cloudwego.io/zh/docs/eino/release_notes_and_migration/eino_v0.9._agentic-runtime/eino_v0.9_migration_notes/)。修改 Eino 代码前必须完整阅读；升级完成后必须再逐项复核一次，不得仅凭编译通过判定迁移完成。
- 每次执行都必须检查 Eino 和其他核心依赖是否已有比本文更新的官方迁移文档，并以执行时的最新稳定文档为准。
- 其他直接依赖必须分别阅读官方 release / migration 文档；不使用非官方二手摘要替代官方说明。

## 升级范围

- 后端 Go 版本、全部直接 Go 依赖和开发工具升级到最新稳定版。
- 前端 Node/npm、全部 dependencies 和 devDependencies 升级到最新稳定版。
- 同步刷新间接依赖、lockfile、Docker 基础镜像、CI 配置和相关工具配置。
- 不使用 alpha、beta、rc 等预发布版本，除非用户明确决定采用。

## 兼容策略

- 无需兼容旧写法、旧 API、旧配置、旧数据、旧数据库结构和旧 migration。
- 允许清空并重建 Demo 数据库、fixture 和索引，不编写历史数据回填或兼容迁移。
- 无需兼容旧测试用例和测试脚本，但必须将其改写为验证新架构和新行为；不得仅删除失败测试来通过验证。
- 删除 deprecated / NOT RECOMMENDED API、兼容 adapter、过渡分支、无效配置和旧实现。
- 如果项目已经进入生产或存在不可丢失数据，执行本 Playbook 前必须由用户重新确认兼容策略，不得默认清库。

## 架构调整要求

- 如果新版本提供官方推荐且更合理的 API、写法或架构，应同步迁移，不只做最小编译修复。
- 优先采用能减少自定义封装、重复状态、兼容代码和维护成本的方案。
- 不采用尚未稳定发布的实验性架构，不重构与本轮依赖升级无关的模块。
- 架构调整必须能指向官方文档、deprecated / NOT RECOMMENDED 说明，或明确的复杂度降低；不以主观的“更现代”为理由无限重写。

## 执行循环

1. 盘点后端、前端、工具链、运行环境的直接与间接依赖。
2. 查询最新稳定版本，完整阅读各直接依赖的官方 release / migration 文档。
3. 按有关联的依赖组分批升级，并同步修改代码、配置、数据结构、fixture、测试和脚本。
4. 运行格式化、依赖整理、后端测试、前端构建、脚本检查和 Demo 主流程，修复全部问题。
5. 再次检查过期依赖、deprecated API、NOT RECOMMENDED 架构、旧数据结构和遗留兼容代码。
6. 只要仍存在可升级稳定依赖或官方已推荐的更新写法，就重复第 2-5 步。
7. 满足全部停止条件后结束当前轮次，不继续追逐版本或进行无依据的架构重写。

## 停止条件

- 所有直接依赖均为执行当时的最新稳定版，间接依赖已由包管理器重新解析，最后复查无遗漏。
- 项目不再使用官方已 deprecated 或 NOT RECOMMENDED 的 API / 架构，不再保留本轮可删除的历史兼容层。
- `cd backend && go mod tidy && go test ./...` 通过。
- `cd frontend && npm install && npm run build` 通过，并提交更新后的 lockfile。
- `bash -n scripts/*.sh` 通过。
- 固定 fake provider 配置下 `bash scripts/demo_bootstrap.sh fresh` 和核心生成链路通过。
- 升级完成后重新打开本轮阅读过的官方迁移文档逐项复核，并再检查一次所有依赖版本。
- 本轮版本基线、架构变化、验证结果、遗留风险和 commit 已记录到 `docs/regression/dependency_architecture_refresh_<date>.md`。

## 每轮记录模板

```markdown
# 依赖与架构基线刷新 - YYYY-MM-DD

## 版本基线

- Go:
- Node/npm:
- Eino:
- 后端核心依赖:
- 前端核心依赖:

## 官方文档

- 已阅读:
- 关键迁移项:

## 架构变化

- 移除的旧写法/兼容层:
- 采用的新 API/架构:

## 验证

- 后端测试:
- 前端构建:
- 脚本检查:
- Demo fresh/生成链路:

## 遗留风险

- None / ...

## Git

- Commit:
```
