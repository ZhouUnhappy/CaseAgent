package knowledge

import (
	"strings"
	"testing"
)

func TestBuildDraftPromptRequiresHumanReviewLanguage(t *testing.T) {
	prompt, err := BuildDraftPrompt(DraftInput{
		CandidateType: "module",
		CandidateName: "Billing-Core",
		SourceSnippets: []map[string]any{
			{"text": "Billing-Core 需要校验账单明细"},
		},
	})
	if err != nil {
		t.Fatalf("BuildDraftPrompt() returned error: %v", err)
	}

	for _, want := range []string{"Billing-Core", "待人工校对", "不要把无法从来源片段确认的信息写成确定事实"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestStripMarkdownFence(t *testing.T) {
	got := stripMarkdownFence("```markdown\n# Title\nbody\n```")
	if got != "# Title\nbody" {
		t.Fatalf("stripMarkdownFence() = %q", got)
	}
}
