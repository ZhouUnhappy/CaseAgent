package handler

import (
	"strings"
	"testing"
	"time"
)

func TestDemoRunToken(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 9, 8, 0, time.UTC)
	if got := demoRunToken(" custom-run ", now); got != "custom-run" {
		t.Fatalf("expected custom run token, got %q", got)
	}
	if got := demoRunToken("", now); got != "demo-20260611100908" {
		t.Fatalf("unexpected generated run token: %q", got)
	}
}

func TestDemoFrontendURL(t *testing.T) {
	if got := demoFrontendURL("http://localhost:40002/", "/tasks/12"); got != "http://localhost:40002/tasks/12" {
		t.Fatalf("unexpected absolute URL: %q", got)
	}
	if got := demoFrontendURL("", "projects/7"); got != "/projects/7" {
		t.Fatalf("unexpected path URL: %q", got)
	}
}

func TestPublicDemoFixturePathRejectsPrivateTraversal(t *testing.T) {
	if _, err := publicDemoFixturePath("../private/i1_private.env"); err == nil {
		t.Fatal("expected path traversal fixture name to be rejected")
	}
	if _, err := publicDemoFixturePath("private/secret.md"); err == nil {
		t.Fatal("expected nested fixture name to be rejected")
	}
}

func TestReadPublicDemoFixture(t *testing.T) {
	content, err := readPublicDemoFixture(demoRequirementFixture)
	if err != nil {
		t.Fatalf("readPublicDemoFixture returned error: %v", err)
	}
	if !strings.Contains(content, "probe-gate-7781") {
		t.Fatalf("fixture content does not look like public demo requirement: %.80q", content)
	}
}
