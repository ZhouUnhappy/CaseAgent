package suggestion

import (
	"testing"
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
