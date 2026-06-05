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
			Agent: newCaseGenerationADKAgent("broken", "broken test agent", func(context.Context, string, string) (string, error) {
				return "", errors.New("provider failed")
			}),
		},
		{
			Name: "functional",
			Agent: newCaseGenerationADKAgent("functional", "functional test agent", func(context.Context, string, string) (string, error) {
				return `[{"section":"功能测试","cases":[{"title":"ok"}]}]`, nil
			}),
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
			Agent: newCaseGenerationADKAgent("bad-json", "bad JSON test agent", func(context.Context, string, string) (string, error) {
				return `not json`, nil
			}),
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

func TestAgentGraphRecordsNodeMetadata(t *testing.T) {
	recorder := &fakeAgentTraceRecorder{}
	graph := newAgentGraph([]agentGraphNode{
		{
			Name: "functional",
			Agent: newCaseGenerationADKAgent("functional", "functional test agent", func(context.Context, string, string) (string, error) {
				return `[{"section":"功能测试","cases":[{"title":"ok"}]}]`, nil
			}),
		},
	}, time.Second, recorder)

	result := graph.Run(context.Background(), "requirements", "knowledge")
	if len(result.Sections) != 1 {
		t.Fatalf("sections = %#v, want one", result.Sections)
	}
	if len(recorder.startedAgents) != 1 {
		t.Fatalf("started agent runs = %#v, want one", recorder.startedAgents)
	}
	started := recorder.startedAgents[0]
	if started.Metadata["graph_node"] != "functional" || started.Metadata["graph_node_type"] != "sub_agent" {
		t.Fatalf("start metadata = %#v, want graph node metadata", started.Metadata)
	}
	if len(recorder.finishedAgents) != 1 {
		t.Fatalf("finished agent runs = %#v, want one", recorder.finishedAgents)
	}
	if recorder.finishedAgents[0].Metadata["elapsed_ms"] == nil {
		t.Fatalf("finish metadata = %#v, want elapsed_ms", recorder.finishedAgents[0].Metadata)
	}
}
