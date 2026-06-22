package prompts

import (
	"strings"
	"testing"
)

func TestRegistrySelectsDefaultVersion(t *testing.T) {
	registry, err := NewRegistry([]Template{
		{ID: "demo.prompt", Version: "v1", Body: "old {{.Name}}"},
		{ID: "demo.prompt", Version: "v2", Body: "new {{.Name}}", Default: true},
	})
	if err != nil {
		t.Fatalf("NewRegistry() returned error: %v", err)
	}

	rendered, err := registry.Render("demo.prompt", map[string]string{"Name": "case"})
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}
	if rendered.Version != "v2" {
		t.Fatalf("default version = %q, want v2", rendered.Version)
	}
	if rendered.Content != "new case" {
		t.Fatalf("content = %q", rendered.Content)
	}
}

func TestRegistryMissingVersionReturnsError(t *testing.T) {
	registry, err := NewRegistry([]Template{
		{ID: "demo.prompt", Version: "v1", Body: "body", Default: true},
	})
	if err != nil {
		t.Fatalf("NewRegistry() returned error: %v", err)
	}

	_, err = registry.RenderVersion("demo.prompt", "v404", nil)
	if err == nil || !strings.Contains(err.Error(), "demo.prompt") || !strings.Contains(err.Error(), "v404") {
		t.Fatalf("RenderVersion() error = %v, want missing version", err)
	}
}

func TestRegistryDefaultVersionsReturnsCopy(t *testing.T) {
	registry, err := NewRegistry([]Template{
		{ID: "demo.prompt", Version: "v1", Body: "old {{.Name}}"},
		{ID: "demo.prompt", Version: "v2", Body: "new {{.Name}}", Default: true},
	})
	if err != nil {
		t.Fatalf("NewRegistry() returned error: %v", err)
	}

	versions := registry.DefaultVersions()
	if versions["demo.prompt"] != "v2" {
		t.Fatalf("DefaultVersions() = %#v, want demo.prompt v2", versions)
	}
	versions["demo.prompt"] = "mutated"
	if registry.DefaultVersions()["demo.prompt"] != "v2" {
		t.Fatalf("DefaultVersions returned internal map")
	}
}

func TestDefaultPromptsKeepFakeProviderSentinels(t *testing.T) {
	cases := []struct {
		id     ID
		needle string
	}{
		{FunctionalCases, "功能与输入域测试专家"},
		{OpsCases, "运维测试专家"},
		{FailureCases, "故障测试专家"},
		{DeepCases, "测试用例生成专家"},
	}

	for _, tc := range cases {
		rendered, err := DefaultRegistry().Render(tc.id, CasePromptData{
			Requirements: "requirements",
			Knowledge:    "knowledge",
		})
		if err != nil {
			t.Fatalf("Render(%s) returned error: %v", tc.id, err)
		}
		if !strings.Contains(rendered.Content, tc.needle) {
			t.Fatalf("Render(%s) missing fake sentinel %q:\n%s", tc.id, tc.needle, rendered.Content)
		}
	}
}

func TestDefaultCasePromptsDistinguishEnumsFromBoundaries(t *testing.T) {
	registry := DefaultRegistry()
	data := CasePromptData{Requirements: "sizes: S/M/L/XL", Knowledge: "knowledge"}

	for _, id := range []ID{FunctionalCases, DeepCases} {
		rendered, err := registry.Render(id, data)
		if err != nil {
			t.Fatalf("Render(%s) returned error: %v", id, err)
		}
		wantVersion := "v2"
		if id == FunctionalCases {
			wantVersion = "v3"
		}
		if rendered.Version != wantVersion {
			t.Fatalf("Render(%s) version = %q, want %s", id, rendered.Version, wantVersion)
		}
		if !strings.Contains(rendered.Content, "S/M/L/XL") {
			t.Fatalf("Render(%s) missing enum classification guidance", id)
		}
	}
}
