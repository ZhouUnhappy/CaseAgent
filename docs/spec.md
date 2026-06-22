# CaseAgent 规格说明

## 文档定位

这份文档描述 CaseAgent 的稳定业务规格，用来回答“系统应该做什么、输入输出长什么样”。执行顺序、阶段状态和当前优先级不写在这里；当前架构导航见 [`architecture.md`](architecture.md)，后续优化见 [`future_work.md`](future_work.md)。

## 前提与范围

- 当前需求输入以文字描述为主，不依赖额外图片理解。
- 系统目标是基于需求文档和架构知识生成可审核的测试用例。
- 知识库当前分为两类：`product` 和 `module`。

## 核心业务对象

- `tenants`：租户隔离边界（部门 / 公司）。所有业务对象都强归属于一个 tenant，跨 tenant 数据互不可见（由 PostgreSQL RLS 强制；详见 [`multitenancy.md`](multitenancy.md)）。
- `projects`：项目空间，承载需求文档、任务和测试用例。
- `documents`：需求文档原始记录，可来自 md 文件或 Google Drive。
- `document_chunks`：需求文档分块后的检索单元。
- `knowledge_base`：架构知识记录，按 `product` / `module` 分类管理。
- `case_generation_tasks`：测试用例生成任务。
- `test_cases`：按任务保存的测试用例结果。

## 多 Agent 职责划分

### Agent Service

- 调用功能、运维和故障 3 个生成 Agent。
- 解析结构化输出，在后端完成同名分类合并、全局去重和稳定排序。
- 单个 Agent 失败不阻塞其他结果；全部 Agent 无可用结果时才调用 DeepAgent 生成一次完整结果。

### 子 Agent

- 功能测试 Agent：按模块生成功能流程、业务规则、输入校验和有需求依据的边界值场景；边界测试是输出分类，不是独立 Agent。
- 运维测试 Agent：生成升级、扩容、缩容等运维场景用例。
- 故障测试 Agent：生成节点重启、掉电等故障场景用例。

### DeepAgent

- 只在 3 个生成 Agent 全部失败或没有可解析结果时使用。
- 直接根据需求和知识生成完整分类结果，不重复启动子 Agent。

## 知识库文档规范

### 产品文档（`product`）

建议包含以下信息：

- 概述：产品简介、产品线、支持环境。
- 整体架构：组件关系、服务启动依赖。
- 相关服务和端口。
- 日志路径。
- E2E 测试关注点。

### 模块文档（`module`）

建议包含以下信息：

- 概述：模块功能描述。
- 核心功能：功能点列表。
- 工作原理：工作流程。
- 依赖关系：依赖组件说明。
- 与其他功能的关系。
- E2E 测试关注点：基础功能、异常场景、组合场景、边界情况。

## 测试用例输出契约

系统生成结果按 section 分组，每组包含一批 cases。目标 JSON 结构如下：

```json
{
  "section": "模块名/子模块名",
  "cases": [
    {
      "title": "[模块] 用例标题",
      "priority_id": 3,
      "custom_preconds": "前置条件",
      "custom_steps_separated": [
        {
          "content": "步骤1",
          "expected": "预期结果1"
        },
        {
          "content": "步骤2",
          "expected": "预期结果2"
        }
      ]
    }
  ]
}
```

`priority_id` 约定如下：

- `1` = Low
- `2` = Medium
- `3` = High
- `4` = Critical

## 主流程规格

**前置**：选择或创建租户（前端顶栏；API 通过 `X-Tenant-ID` header 传 slug，由 `middleware/tenant.go` 解析）。所有后续步骤自动绑定当前 tenant 上下文，RLS 在 DB 层强制隔离。

1. 创建项目。
2. 上传需求文档，支持 md 文件或 Google Drive ID。
3. 对需求文档进行清洗、分块、向量化和存储。
4. 维护知识库文档。
5. 创建生成任务，识别受影响的产品和模块。
6. 用户审核受影响范围。
7. 基于需求和知识上下文生成测试用例。
8. 用户审核、修改并提交测试用例。
9. 系统视情况给出知识库更新建议，用户决定是否确认更新。

## 与其他文档的分工

- `docs/spec.md`：记录稳定业务规格、输入输出约束和角色职责。
- `docs/architecture.md`：记录当前代码入口、验证脚本和回归证据索引。
- `docs/future_work.md`：记录尚未启动、需要触发条件的后续优化方向。
