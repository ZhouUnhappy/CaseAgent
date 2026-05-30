package suggestion

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// CandidateMatch 表示一个候选词在 requirements 中的出现情况。
type CandidateMatch struct {
	Name      string
	Frequency int
	Snippets  []string
}

// 英文标识符：含连字符 / 下划线 的复合 token，例：Module-X / billing_core / Probe-Gate-7781
var reCompoundIdent = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:[-_][A-Za-z0-9]+)+`)

// 全大写缩写（2-6 字符），例：API / PDF / RBAC
var reAcronym = regexp.MustCompile(`\b[A-Z]{2,6}\b`)

// CamelCase（至少两段），例：BillingCore / UserService
var reCamelCase = regexp.MustCompile(`\b[A-Z][a-z]+(?:[A-Z][a-z]+){1,3}\b`)

// 中文复合实体：定位白名单后缀，再向前取一小段连续实体字符。
var reChineseEntitySuffix = regexp.MustCompile(`模块|服务|组件|系统|平台|核心|网关|引擎|中心`)

var chineseEntitySuffixes = []string{"模块", "服务", "组件", "系统", "平台", "核心", "网关", "引擎", "中心"}

var chineseLeadingStopPhrases = []string{
	"本次需求", "本次", "需求", "涉及", "覆盖", "校验", "验证", "需要",
	"新增", "升级", "改造", "支持", "调用", "通过", "依赖", "以及",
	"同时", "并且", "其中", "针对", "关于", "和", "与", "的",
}

var chineseGenericCandidates = map[string]struct{}{
	"本模块": {}, "该模块": {}, "此模块": {},
	"本服务": {}, "该服务": {}, "此服务": {},
	"本组件": {}, "该组件": {}, "此组件": {},
	"本系统": {}, "该系统": {}, "此系统": {},
}

// minFrequency: 候选最少出现次数。
const minFrequency = 2

// maxSnippets: 每个候选最多保留多少条上下文片段
const maxSnippets = 3

// ExtractCandidates 在 requirements 文本中提取标识符候选。
//
// 英文识别覆盖连字符/下划线复合、全大写缩写、CamelCase。中文识别使用
// 白名单后缀方案抓取「X模块 / Y服务 / Z组件」这类常见复合名，不引入
// 分词依赖；为降低误报，会剥离常见动词前缀并过滤「本模块」等泛称。
//
// exclude 集合（normalize 后比较）中的候选会被丢弃 —— 通常传入 analyze
// 阶段已经 inferred 的 products + modules，避免把已识别的当作"缺失"。
//
// 调用方拿到结果后，可以再用 retrieval 查 knowledge top-1 score 决定哪些
// 候选真正算"缺失"。本函数本身不依赖数据库，便于单测。
func ExtractCandidates(requirements string, exclude []string) []CandidateMatch {
	if strings.TrimSpace(requirements) == "" {
		return nil
	}

	excludeSet := make(map[string]struct{}, len(exclude))
	for _, name := range exclude {
		excludeSet[normalize(name)] = struct{}{}
	}

	type info struct {
		name    string
		count   int
		offsets []int
	}
	bag := make(map[string]*info)

	add := func(canonical string, offset int) {
		if _, skip := excludeSet[normalize(canonical)]; skip {
			return
		}
		entry, ok := bag[canonical]
		if !ok {
			entry = &info{name: canonical}
			bag[canonical] = entry
		}
		entry.count++
		if len(entry.offsets) < maxSnippets {
			entry.offsets = append(entry.offsets, offset)
		}
	}

	for _, m := range reCompoundIdent.FindAllStringIndex(requirements, -1) {
		add(requirements[m[0]:m[1]], m[0])
	}
	for _, m := range reAcronym.FindAllStringIndex(requirements, -1) {
		add(requirements[m[0]:m[1]], m[0])
	}
	for _, m := range reCamelCase.FindAllStringIndex(requirements, -1) {
		add(requirements[m[0]:m[1]], m[0])
	}
	for _, c := range extractChineseCandidates(requirements) {
		add(c.name, c.offset)
	}

	results := make([]CandidateMatch, 0, len(bag))
	for _, e := range bag {
		if e.count < minFrequency {
			continue
		}
		results = append(results, CandidateMatch{
			Name:      e.name,
			Frequency: e.count,
			Snippets:  collectSnippets(requirements, e.offsets, e.name),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Frequency != results[j].Frequency {
			return results[i].Frequency > results[j].Frequency
		}
		return results[i].Name < results[j].Name
	})

	return results
}

func collectSnippets(text string, offsets []int, name string) []string {
	if len(offsets) == 0 {
		return nil
	}
	runes := []rune(text)
	out := make([]string, 0, len(offsets))
	for _, byteOffset := range offsets {
		runeStart := byteOffsetToRune(text, byteOffset)
		windowStart := runeStart - 30
		if windowStart < 0 {
			windowStart = 0
		}
		windowEnd := runeStart + len([]rune(name)) + 30
		if windowEnd > len(runes) {
			windowEnd = len(runes)
		}
		snippet := strings.TrimSpace(string(runes[windowStart:windowEnd]))
		out = append(out, snippet)
	}
	return out
}

func byteOffsetToRune(text string, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset >= len(text) {
		return len([]rune(text))
	}
	return len([]rune(text[:byteOffset]))
}

func normalize(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

type chineseCandidate struct {
	name   string
	offset int
}

func extractChineseCandidates(text string) []chineseCandidate {
	matches := reChineseEntitySuffix.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}

	out := make([]chineseCandidate, 0, len(matches))
	for _, m := range matches {
		prefixText := text[:m[0]]
		prefixRunes := []rune(prefixText)
		start := len(prefixRunes)
		for start > 0 && len(prefixRunes)-start < 12 && isChineseEntityRune(prefixRunes[start-1]) {
			start--
		}
		prefix := string(prefixRunes[start:])
		raw := prefix + text[m[0]:m[1]]
		name := normalizeChineseCandidate(raw)
		if name == "" {
			continue
		}
		out = append(out, chineseCandidate{
			name:   name,
			offset: m[0] - len([]byte(prefix)),
		})
	}
	return out
}

func isChineseEntityRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func normalizeChineseCandidate(value string) string {
	candidate := strings.TrimSpace(value)
	for {
		trimmed := candidate
		for _, prefix := range chineseLeadingStopPhrases {
			trimmed = strings.TrimPrefix(trimmed, prefix)
		}
		trimmed = strings.TrimLeft(trimmed, "的一二三四五六七八九十、，。；：:;,. ")
		if trimmed == candidate {
			break
		}
		candidate = strings.TrimSpace(trimmed)
	}
	for _, sep := range []string{"以及", "并且", "和", "与", "及"} {
		if idx := strings.LastIndex(candidate, sep); idx >= 0 {
			candidate = strings.TrimSpace(candidate[idx+len(sep):])
		}
	}

	candidate = trimChineseCandidateLength(candidate)
	if _, generic := chineseGenericCandidates[candidate]; generic {
		return ""
	}
	if len([]rune(candidate)) < 3 {
		return ""
	}
	return candidate
}

func trimChineseCandidateLength(candidate string) string {
	runes := []rune(candidate)
	if len(runes) <= 12 {
		return candidate
	}
	for _, suffix := range chineseEntitySuffixes {
		suffixRunes := []rune(suffix)
		if !strings.HasSuffix(candidate, suffix) || len(runes) <= len(suffixRunes) {
			continue
		}
		prefix := runes[:len(runes)-len(suffixRunes)]
		if len(prefix) > 8 {
			prefix = prefix[len(prefix)-8:]
		}
		return string(prefix) + suffix
	}
	return string(runes[len(runes)-12:])
}
