package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAgentGraphContinuesAfterNodeFailure(t *testing.T) {
	graph := newAgentGraph([]agentGraphNode{
		{
			Name: "broken",
			Run: func(context.Context, string, string) (string, error) {
				return "", errors.New("provider failed")
			},
		},
		{
			Name: "functional",
			Run: func(context.Context, string, string) (string, error) {
				return `[{"section":"功能测试","cases":[{"title":"ok"}]}]`, nil
			},
		},
	}, time.Second, nil)

	result := graph.Run(context.Background(), "requirements", "knowledge")
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(result.Nodes))
	}
	if result.Nodes[0].Err == nil {
		t.Fatal("first node expected error")
	}
	if len(result.Sections) != 1 || result.Sections[0].Section != "功能测试" {
		t.Fatalf("sections = %#v", result.Sections)
	}
}

func TestAgentGraphSkipsUnparseableOutput(t *testing.T) {
	graph := newAgentGraph([]agentGraphNode{
		{
			Name: "bad-json",
			Run: func(context.Context, string, string) (string, error) {
				return `not json`, nil
			},
		},
	}, time.Second, nil)

	result := graph.Run(context.Background(), "requirements", "knowledge")
	if len(result.Sections) != 0 {
		t.Fatalf("sections = %#v, want none", result.Sections)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ParseErr == nil {
		t.Fatalf("node result = %#v, want parse error", result.Nodes)
	}
}
