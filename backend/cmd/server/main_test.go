package main

import (
	"os"
	"testing"
)

func TestResolveConfigPathUsesEnvOverride(t *testing.T) {
	t.Setenv("CASEAGENT_CONFIG", " /tmp/caseagent.yaml ")

	if got := resolveConfigPath(); got != "/tmp/caseagent.yaml" {
		t.Fatalf("resolveConfigPath() = %q, want env override", got)
	}
}

func TestResolveConfigPathPrefersLocalDotConfig(t *testing.T) {
	chdir(t, t.TempDir())
	t.Setenv("CASEAGENT_CONFIG", "")

	if err := os.Mkdir("configs", 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.WriteFile("configs/.config.yaml", []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	if got := resolveConfigPath(); got != "configs/.config.yaml" {
		t.Fatalf("resolveConfigPath() = %q, want local dot config", got)
	}
}

func TestResolveConfigPathFallsBackToCommittedName(t *testing.T) {
	chdir(t, t.TempDir())
	t.Setenv("CASEAGENT_CONFIG", "")

	if got := resolveConfigPath(); got != "configs/config.yaml" {
		t.Fatalf("resolveConfigPath() = %q, want fallback config", got)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
