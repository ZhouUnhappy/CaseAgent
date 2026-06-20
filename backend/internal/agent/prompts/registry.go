package prompts

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

type ID string

const (
	FunctionalCases ID = "agent.functional.cases"
	OpsCases        ID = "agent.ops.cases"
	FailureCases    ID = "agent.failure.cases"
	BoundaryCases   ID = "agent.boundary.cases"
	DeepCases       ID = "agent.deep.cases"
	DeepRefineCases ID = "agent.deep.refine_cases"
)

type CasePromptData struct {
	Requirements string
	Knowledge    string
	DraftJSON    string
}

type Template struct {
	ID      ID
	Version string
	Body    string
	Default bool
}

type Rendered struct {
	ID      ID
	Version string
	Content string
}

type Registry struct {
	templates map[ID]map[string]Template
	defaults  map[ID]string
}

type traceContextKey struct{}

type TraceInfo struct {
	ID      ID
	Version string
}

func NewRegistry(templates []Template) (*Registry, error) {
	registry := &Registry{
		templates: map[ID]map[string]Template{},
		defaults:  map[ID]string{},
	}
	for _, item := range templates {
		item.ID = ID(strings.TrimSpace(string(item.ID)))
		item.Version = strings.TrimSpace(item.Version)
		if item.ID == "" {
			return nil, fmt.Errorf("prompt template id is required")
		}
		if item.Version == "" {
			return nil, fmt.Errorf("prompt template %q version is required", item.ID)
		}
		if strings.TrimSpace(item.Body) == "" {
			return nil, fmt.Errorf("prompt template %q@%s body is required", item.ID, item.Version)
		}

		versions := registry.templates[item.ID]
		if versions == nil {
			versions = map[string]Template{}
			registry.templates[item.ID] = versions
		}
		if _, exists := versions[item.Version]; exists {
			return nil, fmt.Errorf("duplicate prompt template %q@%s", item.ID, item.Version)
		}
		versions[item.Version] = item

		if item.Default {
			if existing := registry.defaults[item.ID]; existing != "" {
				return nil, fmt.Errorf("multiple default prompt versions for %q: %s and %s", item.ID, existing, item.Version)
			}
			registry.defaults[item.ID] = item.Version
		}
	}
	for id, versions := range registry.templates {
		if registry.defaults[id] == "" {
			registry.defaults[id] = latestVersion(versions)
		}
	}
	return registry, nil
}

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func (r *Registry) DefaultVersions() map[ID]string {
	if r == nil {
		r = DefaultRegistry()
	}
	versions := make(map[ID]string, len(r.defaults))
	for id, version := range r.defaults {
		versions[id] = version
	}
	return versions
}

func (r *Registry) Render(id ID, data any) (Rendered, error) {
	if r == nil {
		r = DefaultRegistry()
	}
	version, ok := r.defaults[id]
	if !ok {
		return Rendered{}, fmt.Errorf("prompt template %q not found", id)
	}
	return r.RenderVersion(id, version, data)
}

func (r *Registry) RenderVersion(id ID, version string, data any) (Rendered, error) {
	if r == nil {
		r = DefaultRegistry()
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return Rendered{}, fmt.Errorf("prompt template %q version is required", id)
	}
	versions := r.templates[id]
	if versions == nil {
		return Rendered{}, fmt.Errorf("prompt template %q not found", id)
	}
	item, ok := versions[version]
	if !ok {
		return Rendered{}, fmt.Errorf("prompt template %q@%s not found", id, version)
	}

	tmpl, err := template.New(string(id)).Option("missingkey=error").Parse(item.Body)
	if err != nil {
		return Rendered{}, fmt.Errorf("parse prompt template %q@%s: %w", id, version, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return Rendered{}, fmt.Errorf("render prompt template %q@%s: %w", id, version, err)
	}
	return Rendered{ID: id, Version: version, Content: buf.String()}, nil
}

func WithRenderedPrompt(ctx context.Context, rendered Rendered) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceContextKey{}, TraceInfo{
		ID:      rendered.ID,
		Version: rendered.Version,
	})
}

func TraceFromContext(ctx context.Context) (TraceInfo, bool) {
	if ctx == nil {
		return TraceInfo{}, false
	}
	info, ok := ctx.Value(traceContextKey{}).(TraceInfo)
	return info, ok && info.ID != "" && info.Version != ""
}

func latestVersion(versions map[string]Template) string {
	keys := make([]string, 0, len(versions))
	for version := range versions {
		keys = append(keys, version)
	}
	sort.Strings(keys)
	return keys[len(keys)-1]
}

var defaultRegistry = mustDefaultRegistry()

func mustDefaultRegistry() *Registry {
	registry, err := NewRegistry([]Template{
		{ID: FunctionalCases, Version: "v1", Body: functionalCasesV1},
		{ID: FunctionalCases, Version: "v2", Body: functionalCasesV2, Default: true},
		{ID: OpsCases, Version: "v1", Body: opsCasesV1, Default: true},
		{ID: FailureCases, Version: "v1", Body: failureCasesV1, Default: true},
		{ID: BoundaryCases, Version: "v1", Body: boundaryCasesV1},
		{ID: BoundaryCases, Version: "v2", Body: boundaryCasesV2, Default: true},
		{ID: DeepCases, Version: "v1", Body: deepCasesV1},
		{ID: DeepCases, Version: "v2", Body: deepCasesV2, Default: true},
		{ID: DeepRefineCases, Version: "v1", Body: deepRefineCasesV1},
		{ID: DeepRefineCases, Version: "v2", Body: deepRefineCasesV2, Default: true},
	})
	if err != nil {
		panic(err)
	}
	return registry
}

const functionalCasesV1 = `你是一个功能测试专家。根据以下需求和相关知识，只生成功能测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "功能测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`

const functionalCasesV2 = `你是一个功能测试专家。根据以下需求和相关知识，只生成功能测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

分类规则：
- 需求明确支持的正常枚举值（例如 S/M/L/XL）逐一验证属于功能测试。
- 不要仅因枚举值位于列表首尾，就将其称为边界值。

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "功能测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`

const opsCasesV1 = `你是一个运维测试专家。根据以下需求和相关知识，只生成运维测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

请重点关注：
- 升级场景
- 扩容场景
- 维护场景

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "运维测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`

const failureCasesV1 = `你是一个故障测试专家。根据以下需求和相关知识，只生成故障测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

请重点关注：
- 节点重启
- 断电恢复
- 网络分区

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "故障测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`

const boundaryCasesV1 = `你是一个边界测试专家。根据以下需求和相关知识，只生成边界测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

请重点关注：
- 参数边界值
- 边缘情况
- 无效输入

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "边界测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`

const boundaryCasesV2 = `你是一个边界测试专家。根据以下需求和相关知识，只生成边界测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

边界测试只关注：
- 可比较的最小值、最大值、阈值以及刚好越界的值。
- 长度、容量、数量、尺寸、时间、格式等输入域的边界和无效输入。
- 空值、缺失值、极端值等真正的边缘情况。

分类限制：
- 需求明确支持的正常枚举值（例如 S/M/L/XL）逐一验证属于功能测试，不属于边界测试。
- 只有当某个枚举项明确代表可比较的最小或最大限制时，才能作为边界。

请只返回如下结构的 JSON 数组，不要解释文字，不要 Markdown 代码块：
[
  {
    "section": "边界测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]`

const deepCasesV1 = `你是一个测试用例生成专家。根据以下需求和相关知识，输出结构化测试用例。

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

请生成 JSON 数组，每个元素表示一个 section，结构必须严格如下：
[
  {
    "section": "功能测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"},
          {"content": "步骤2", "expected": "预期2"}
        ]
      }
    ]
  }
]

要求：
1. 功能测试
2. 运维测试
3. 故障测试
4. 边界测试
5. priority_id 取值 1-4，默认高优先级可用 3
6. custom_steps_separated 中每一步都必须包含 content 和 expected
7. 只返回合法 JSON，不要 Markdown 代码块，不要解释文字`

const deepCasesV2 = deepCasesV1 + `
8. 正常枚举值全覆盖属于功能测试；只有可比较阈值、刚好越界值和无效输入属于边界测试。`

const deepRefineCasesV1 = `你是测试用例总协调 Agent。请基于以下输入，对草稿用例做统一整理：

需求:
{{.Requirements}}

相关知识:
{{.Knowledge}}

子 Agent 草稿（JSON）:
{{.DraftJSON}}

请输出最终 JSON 数组，结构必须是：
[
  {
    "section": "功能测试",
    "cases": [
      {
        "title": "[模块] 用例标题",
        "priority_id": 3,
        "custom_preconds": "前置条件",
        "custom_steps_separated": [
          {"content": "步骤1", "expected": "预期1"}
        ]
      }
    ]
  }
]

要求：
1. 合并重复用例，保留信息更完整的版本。
2. 每个 case 必须包含 title/priority_id/custom_preconds/custom_steps_separated。
3. custom_steps_separated 每一步都必须有 content 与 expected。
4. 仅返回合法 JSON，不要 Markdown 代码块，不要解释文字。`

const deepRefineCasesV2 = deepRefineCasesV1 + `
5. 校正 section 分类：正常支持的枚举值（例如 S/M/L/XL）全覆盖归入功能测试；只有可比较阈值、刚好越界值和无效输入归入边界测试。`
