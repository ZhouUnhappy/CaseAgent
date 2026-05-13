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

// minFrequency: 英文候选最少出现次数。
const minFrequency = 2

// maxSnippets: 每个候选最多保留多少条上下文片段
const maxSnippets = 3

// ExtractCandidates 在 requirements 文本中提取**英文**标识符候选。
//
// MVP 阶段仅识别英文标识符（含连字符/下划线复合、全大写缩写、CamelCase），
// 这是当前 fixture / 真实知识库 (name 列) 主要使用的命名风格。中文实体
// 识别需要分词或词典支持，本函数不处理，避免贪婪正则把多个词吃成一个
// 假候选。
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
