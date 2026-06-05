package task

import (
	"encoding/json"
	"fmt"
	"strings"
)

type generatedSection struct {
	Section string           `json:"section"`
	Cases   []map[string]any `json:"cases"`
}

func parseGeneratedSections(raw string) ([]generatedSection, error) {
	cleaned := extractJSONPayload(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("empty model response")
	}

	var sections []generatedSection
	if err := json.Unmarshal([]byte(cleaned), &sections); err == nil && len(sections) > 0 {
		if normalized := normalizeSections(sections); len(normalized) > 0 {
			return normalized, nil
		}
	}

	var singleSection generatedSection
	if err := json.Unmarshal([]byte(cleaned), &singleSection); err == nil && (singleSection.Section != "" || len(singleSection.Cases) > 0) {
		if normalized := normalizeSections([]generatedSection{singleSection}); len(normalized) > 0 {
			return normalized, nil
		}
	}

	var flatCases []map[string]any
	if err := json.Unmarshal([]byte(cleaned), &flatCases); err == nil && len(flatCases) > 0 {
		return groupFlatCases(flatCases), nil
	}

	return nil, fmt.Errorf("response is not valid sectioned or flat JSON")
}

func normalizeSections(sections []generatedSection) []generatedSection {
	normalized := make([]generatedSection, 0, len(sections))
	for _, section := range sections {
		name := strings.TrimSpace(section.Section)
		if name == "" {
			name = "未分类"
		}

		cases := make([]map[string]any, 0, len(section.Cases))
		for _, item := range section.Cases {
			if len(item) == 0 {
				continue
			}
			cases = append(cases, normalizeCase(item, name))
		}

		if len(cases) == 0 {
			continue
		}

		normalized = append(normalized, generatedSection{
			Section: name,
			Cases:   cases,
		})
	}

	return normalized
}

func groupFlatCases(flatCases []map[string]any) []generatedSection {
	grouped := make(map[string][]map[string]any)
	order := make([]string, 0, len(flatCases))

	for _, item := range flatCases {
		section := firstNonEmptyString(item["section"], item["type"])
		if section == "" {
			section = "未分类"
		}

		if _, ok := grouped[section]; !ok {
			order = append(order, section)
		}
		grouped[section] = append(grouped[section], normalizeCase(item, section))
	}

	sections := make([]generatedSection, 0, len(grouped))
	for _, section := range order {
		sections = append(sections, generatedSection{
			Section: section,
			Cases:   grouped[section],
		})
	}

	return sections
}

func dedupeGeneratedSections(sections []generatedSection) []generatedSection {
	seen := make(map[string]struct{})
	deduped := make([]generatedSection, 0, len(sections))

	for _, section := range sections {
		filteredCases := make([]map[string]any, 0, len(section.Cases))
		for _, caseItem := range section.Cases {
			signature := caseSignature(caseItem)
			if signature == "" {
				continue
			}
			if _, ok := seen[signature]; ok {
				continue
			}
			seen[signature] = struct{}{}
			filteredCases = append(filteredCases, caseItem)
		}

		if len(filteredCases) == 0 {
			continue
		}

		deduped = append(deduped, generatedSection{
			Section: section.Section,
			Cases:   filteredCases,
		})
	}

	return deduped
}

func caseSignature(item map[string]any) string {
	title := normalizeMatchText(stringValue(item["title"]))
	preconds := normalizeMatchText(stringValue(item["custom_preconds"]))
	steps := normalizeStepSignatures(item["custom_steps_separated"])

	if title == "" && len(steps) == 0 {
		return ""
	}

	return title + "|" + preconds + "|" + strings.Join(steps, "||")
}

func attachCaseContext(sections []generatedSection, products []string, modules []string) []generatedSection {
	if len(sections) == 0 {
		return sections
	}

	normalizedProducts := append([]string{}, products...)
	normalizedModules := append([]string{}, modules...)

	result := make([]generatedSection, 0, len(sections))
	for _, section := range sections {
		cases := make([]map[string]any, 0, len(section.Cases))
		for _, item := range section.Cases {
			if item == nil {
				continue
			}

			cloned := make(map[string]any, len(item)+3)
			for key, value := range item {
				cloned[key] = value
			}

			if _, ok := cloned["affected_products"]; !ok {
				cloned["affected_products"] = normalizedProducts
			}
			if _, ok := cloned["affected_modules"]; !ok {
				cloned["affected_modules"] = normalizedModules
			}
			if _, ok := cloned["section"]; !ok {
				cloned["section"] = section.Section
			}

			cases = append(cases, cloned)
		}

		result = append(result, generatedSection{
			Section: section.Section,
			Cases:   cases,
		})
	}

	return result
}

func normalizeStepSignatures(value any) []string {
	normalize := func(step map[string]any) string {
		content := normalizeMatchText(firstNonEmptyString(step["content"], step["step"]))
		expected := normalizeMatchText(firstNonEmptyString(step["expected"], step["result"]))
		return content + "=>" + expected
	}

	signatures := make([]string, 0)

	switch typed := value.(type) {
	case []map[string]any:
		for _, step := range typed {
			signatures = append(signatures, normalize(step))
		}
	case []any:
		for _, raw := range typed {
			stepMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			signatures = append(signatures, normalize(stepMap))
		}
	}

	return signatures
}

func normalizeCase(item map[string]any, fallbackSection string) map[string]any {
	title := stringValue(item["title"])
	if title == "" {
		title = fmt.Sprintf("[%s] 自动生成用例", fallbackSection)
	}

	customPreconds := stringValue(item["custom_preconds"])
	if customPreconds == "" {
		customPreconds = stringValue(item["description"])
	}

	normalized := map[string]any{
		"title":                  title,
		"priority_id":            intValue(item["priority_id"], 3),
		"custom_preconds":        customPreconds,
		"custom_steps_separated": normalizeSteps(item),
	}

	return normalized
}

func normalizeSteps(item map[string]any) []map[string]any {
	if value, ok := item["custom_steps_separated"]; ok {
		if steps := normalizeStepList(value); len(steps) > 0 {
			return steps
		}
	}

	expectedResult := stringValue(item["expected_result"])
	if value, ok := item["steps"]; ok {
		if steps := normalizeSimpleSteps(value, expectedResult); len(steps) > 0 {
			return steps
		}
	}

	description := stringValue(item["description"])
	if description != "" || expectedResult != "" {
		return []map[string]any{{
			"content":  firstNonEmptyString(description, "执行测试步骤"),
			"expected": firstNonEmptyString(expectedResult, "结果符合预期"),
		}}
	}

	return []map[string]any{{
		"content":  "执行测试步骤",
		"expected": "结果符合预期",
	}}
}

func normalizeStepList(value any) []map[string]any {
	rawSteps, ok := value.([]any)
	if !ok {
		return nil
	}

	steps := make([]map[string]any, 0, len(rawSteps))
	for _, raw := range rawSteps {
		stepMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		content := firstNonEmptyString(stepMap["content"], stepMap["step"])
		expected := firstNonEmptyString(stepMap["expected"], stepMap["result"])
		steps = append(steps, map[string]any{
			"content":  firstNonEmptyString(content, "执行测试步骤"),
			"expected": expected,
		})
	}

	return steps
}

func normalizeSimpleSteps(value any, expectedResult string) []map[string]any {
	rawSteps, ok := value.([]any)
	if !ok {
		return nil
	}

	steps := make([]map[string]any, 0, len(rawSteps))
	for idx, raw := range rawSteps {
		switch typed := raw.(type) {
		case string:
			step := map[string]any{
				"content":  typed,
				"expected": "",
			}
			if idx == len(rawSteps)-1 && expectedResult != "" {
				step["expected"] = expectedResult
			}
			steps = append(steps, step)
		case map[string]any:
			step := map[string]any{
				"content":  firstNonEmptyString(typed["content"], typed["step"], "执行测试步骤"),
				"expected": firstNonEmptyString(typed["expected"], typed["result"]),
			}
			if idx == len(rawSteps)-1 && step["expected"] == "" && expectedResult != "" {
				step["expected"] = expectedResult
			}
			steps = append(steps, step)
		}
	}

	return steps
}

func extractJSONPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.Join(lines, "\n")
	}

	trimmed = strings.TrimSpace(trimmed)

	startArray := strings.Index(trimmed, "[")
	startObject := strings.Index(trimmed, "{")
	start := firstPositiveIndex(startArray, startObject)
	if start == -1 {
		return trimmed
	}

	endArray := strings.LastIndex(trimmed, "]")
	endObject := strings.LastIndex(trimmed, "}")
	end := max(endArray, endObject)
	if end <= start {
		return trimmed[start:]
	}

	return strings.TrimSpace(trimmed[start : end+1])
}

func normalizeMatchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float32:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	}
	return fallback
}

func firstPositiveIndex(values ...int) int {
	best := -1
	for _, value := range values {
		if value < 0 {
			continue
		}
		if best == -1 || value < best {
			best = value
		}
	}
	return best
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
