# 实现测试用例生成

## 前提
需求内容已完整使用文字描述，无需额外图片理解

## 架构
- 前端：vue + element-plus
- 后端：golang + eino + eino-ext
- 数据库：postgresql + pgvector

## 核心组件
- 文档处理：eino-ext/document/loader/file + markdown HeaderSplitter
- 向量化：eino-ext/components/embedding + indexer（pgvector）
- 检索：eino/flow/retriever/parent + multiquery
- AI Agent：eino/adk DeepAgent（多 Agent 协同）+ compose

## 多 Agent 协同架构
使用 DeepAgent 协调多个子 Agent，防止上下文不够：

### 子 Agent 分工
- **功能测试 Agent**：生成功能验证用例
- **运维测试 Agent**：生成升级、扩容、缩容等运维场景用例
- **故障测试 Agent**：生成节点重启、掉电等故障场景用例
- **边界测试 Agent**：生成参数边界、异常输入等边界用例

### 协同流程
1. DeepAgent 接收需求文档 + 知识库文档
2. 分析测试范围，拆分为不同测试类型
3. 分发任务到对应子 Agent
4. 子 Agent 并行生成各自领域的测试用例
5. DeepAgent 汇总并去重
6. 输出完整的测试用例集合

## 数据库表
- projects：项目信息
- documents：文档信息
- document_chunks：文档分块（向量检索）
- knowledge_base：知识库（products/modules 架构文档）
- test_cases：测试用例
- case_generation_tasks：生成任务

## 知识库文档格式
### 产品文档（products）
- 概述：产品简介、产品线、支持环境
- 整体架构：组件关系、服务启动依赖
- 相关服务和端口
- 日志路径
- E2E 测试关注点

### 模块文档（modules）
- 概述：模块功能描述
- 核心功能：功能点列表
- 工作原理：工作流程
- 依赖关系：依赖组件说明
- 与其他功能的关系
- E2E 测试关注点：基础功能、异常场景、组合场景、边界情况

## 测试用例格式（JSON）
```json
{
  "section": "模块名/子模块名",
  "cases": [{
    "title": "[模块] 用例标题",
    "priority_id": 3,
    "custom_preconds": "前置条件",
    "custom_steps_separated": [
      {"content": "步骤1", "expected": "预期结果1"},
      {"content": "步骤2", "expected": "预期结果2"}
    ]
  }]
}
```
priority_id: 1=Low, 2=Medium, 3=High, 4=Critical

## 流程
1. 项目管理：创建 project
2. 文档上传：支持 md 文件或 Google Drive ID（`gws drive files export --params '{"fileId": "{id}", "mimeType": "text/markdown"}'`）
3. 文档处理：删除 base64 图片 → 分块 → 向量化 → 存储（parent indexer）
4. 知识库管理：products/modules 架构文档，同样分块向量化
5. 生成任务：AI 分析受影响的 products/modules → 用户审核
6. 用例生成：AI 根据需求+知识库生成 JSON 用例
7. 用户审核：修改用例 → 提交审核
8. 知识库更新：AI 判断是否需要更新知识库 → 用户确认

## 技术实现
```go
// 文档处理
loader := file.NewLoader()
transformer := markdown.NewHeaderSplitter(&markdown.HeaderConfig{
    Headers: map[string]string{"##": "level2", "###": "level3"},
})
embedding := openai.NewEmbedding()
indexer := parent.NewIndexer(ctx, &parent.Config{
    Indexer: pgvectorIndexer,
    Transformer: transformer,
    ParentIDKey: "parent_doc_id",
    SubIDGenerator: generateSubIDs,
})

// 检索
retriever := parent.NewRetriever(ctx, &parent.Config{
    Retriever: pgvectorRetriever,
    ParentIDKey: "parent_doc_id",
    OrigDocGetter: getOriginalDocs,
})

// DeepAgent 多 Agent 协同
deepAgent, _ := deep.New(ctx, &deep.Config{
    ChatModel: chatModel,
    SubAgents: []adk.Agent{
        functionalTestAgent,  // 功能测试
        opsTestAgent,         // 运维测试
        failureTestAgent,     // 故障测试
        boundaryTestAgent,    // 边界测试
    },
    ToolsConfig: adk.ToolsConfig{
        ToolsNodeConfig: compose.ToolsNodeConfig{
            Tools: []tool.BaseTool{retrieverTool},
        },
    },
})
```

## 参考
- testrail-case-generate：文档组织、测试用例格式
- eino：Agent Development Kit, Composition, Flow
- eino-ext：Document, Embedding, Indexer, Model 组件
