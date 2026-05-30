package suggestion

import (
	"strings"
	"testing"

	"caseagent/internal/db/models"
)

func TestExtractCandidates_EnglishIdentifiers(t *testing.T) {
	requirements := `
本次需求覆盖 Billing-Core 与 Export-PDF 两个模块的升级。
Billing-Core 的发票生成接口需要兼容 PDF/A。
Export-PDF 改造后将通过 PDF/A 兼容性校验。
另外 ABC 是已存在的产品（不应再次提示）。
`
	got := ExtractCandidates(requirements, []string{"ABC"})

	want := map[string]int{
		"Billing-Core": 2,
		"Export-PDF":   2,
	}
	for _, c := range got {
		if expected, ok := want[c.Name]; ok {
			if c.Frequency != expected {
				t.Fatalf("candidate %q expected frequency %d, got %d", c.Name, expected, c.Frequency)
			}
			delete(want, c.Name)
		}
	}
	if len(want) > 0 {
		t.Fatalf("missing expected candidates: %+v; got=%+v", want, got)
	}
	for _, c := range got {
		if c.Name == "ABC" {
			t.Fatalf("excluded candidate ABC should not appear: %+v", got)
		}
	}
}

func TestExtractCandidates_ChineseSuffixEntities(t *testing.T) {
	requirements := `
本次需求涉及对账核心和发票核对模块。
对账核心需要接入新的核对规则，发票核对模块需要输出差异明细。
`
	got := ExtractCandidates(requirements, nil)

	want := map[string]int{
		"对账核心":   2,
		"发票核对模块": 2,
	}
	for _, c := range got {
		if expected, ok := want[c.Name]; ok {
			if c.Frequency != expected {
				t.Fatalf("candidate %q expected frequency %d, got %d", c.Name, expected, c.Frequency)
			}
			delete(want, c.Name)
		}
	}
	if len(want) > 0 {
		t.Fatalf("missing expected Chinese candidates: %+v; got=%+v", want, got)
	}
}

func TestExtractCandidates_ChineseStopPrefixTrim(t *testing.T) {
	requirements := `
本次需求涉及对账核心。
需求涉及对账核心的异常恢复。
`
	got := ExtractCandidates(requirements, nil)
	for _, c := range got {
		if c.Name == "对账核心" {
			return
		}
		if strings.Contains(c.Name, "需求涉及") {
			t.Fatalf("candidate should strip generic Chinese prefix, got %+v", got)
		}
	}
	t.Fatalf("expected 对账核心 candidate, got %+v", got)
}

func TestExtractCandidates_ChineseGenericStopWords(t *testing.T) {
	requirements := "本模块需要记录日志，本模块需要暴露指标。"
	got := ExtractCandidates(requirements, nil)
	for _, c := range got {
		if c.Name == "本模块" {
			t.Fatalf("generic Chinese candidate should be filtered: %+v", got)
		}
	}
}

func TestExtractCandidates_MinFrequencyFilter(t *testing.T) {
	requirements := "需求只提到 Module-Once 一次。"
	got := ExtractCandidates(requirements, nil)
	for _, c := range got {
		if c.Name == "Module-Once" {
			t.Fatalf("Module-Once below minFrequency=2 should be filtered: %+v", got)
		}
	}
}

func TestExtractCandidates_SortedByFrequencyThenName(t *testing.T) {
	requirements := `
Aaa-Bbb Aaa-Bbb Aaa-Bbb Ccc-Ddd Ccc-Ddd Eee-Fff Eee-Fff
`
	got := ExtractCandidates(requirements, nil)
	if len(got) < 3 {
		t.Fatalf("expected at least 3 candidates, got %d (%+v)", len(got), got)
	}
	if got[0].Name != "Aaa-Bbb" || got[0].Frequency != 3 {
		t.Fatalf("expected Aaa-Bbb first with freq 3, got %+v", got[0])
	}
	if got[1].Name != "Ccc-Ddd" || got[2].Name != "Eee-Fff" {
		t.Fatalf("expected Ccc-Ddd then Eee-Fff for tied freq, got %+v", got[1:3])
	}
}

func TestNormalize_StripsCaseAndWhitespace(t *testing.T) {
	got := normalize(" Billing-Core ")
	want := "billing-core"
	if got != want {
		t.Fatalf("normalize: want %q got %q", want, got)
	}
}

func TestExtractCandidates_RespectsExcludeNormalization(t *testing.T) {
	requirements := "Billing-Core 出现两次。Billing-Core 再来一次。"
	got := ExtractCandidates(requirements, []string{"billing-core"})
	for _, c := range got {
		if c.Name == "Billing-Core" {
			t.Fatalf("normalize-equivalent exclude should match; got %+v", got)
		}
	}
}

func TestValidateManualSuggestionInput(t *testing.T) {
	valid := ManualSuggestionInput{
		CandidateType: models.SuggestionCandidateProduct,
		CandidateName: "Billing-Core",
		SourceTaskID:  1,
		SourceCaseID:  2,
	}
	if err := ValidateManualSuggestionInput(valid); err != nil {
		t.Fatalf("valid input returned error: %v", err)
	}

	invalid := valid
	invalid.CandidateType = "context_gap"
	if err := ValidateManualSuggestionInput(invalid); err == nil {
		t.Fatal("expected candidate_type validation error")
	}
}
