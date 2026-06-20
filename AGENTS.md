# CaseAgent Agent Guide

本文件适用于整个仓库。开始实现前先阅读根目录 `README.md` 和
`docs/engineering_conventions.md`；涉及待办范围时再阅读
`docs/future_work.md`。

## 实现范围

- 当前产品是可信本地 Demo，优先打磨核心需求文档、知识检索、用例生成与审核流程。
- 前端只保证常规桌面浏览器宽度。窄屏和移动端不属于实现或验收范围；可以保留低成本的换行与溢出保护，但不要为其单独设计或测试。
- `docs/future_work.md` 中“要做”和“暂不做”是明确边界。不要擅自启动“暂不做”事项；完成的事项应从活动计划中移除。
- 沿用现有 Vue、Element Plus、Go、Gin 和 Eino 结构，优先扩展现有数据流与组件，不建立平行实现。

## 验证

- 后端变更运行：`cd backend && go test ./...`
- 前端变更运行：`cd frontend && npm run build`
- 只运行与当前改动风险相匹配的额外验证；不要把窄屏检查加入当前验收。

## Git

- 不覆盖或回退用户已有改动。
- 在功能达到可验证边界后提交；用户要求推送时同步推送当前分支。
