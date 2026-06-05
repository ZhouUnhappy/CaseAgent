package agent

import (
	"context"
	"log/slog"
	"time"

	workflowservice "caseagent/internal/service/workflow"
)

type agentGraphNode struct {
	Name string
	Run  func(context.Context, string, string) (string, error)
}

type agentGraph struct {
	nodes    []agentGraphNode
	timeout  time.Duration
	recorder workflowservice.AgentTraceRecorder
}

type agentGraphResult struct {
	Sections []generatedSection
	Nodes    []agentGraphNodeResult
}

type agentGraphNodeResult struct {
	Name         string
	Output       string
	Sections     []generatedSection
	Err          error
	ParseErr     error
	SkippedEmpty bool
}

func newAgentGraph(nodes []agentGraphNode, timeout time.Duration, recorder workflowservice.AgentTraceRecorder) *agentGraph {
	return &agentGraph{
		nodes:    append([]agentGraphNode{}, nodes...),
		timeout:  timeout,
		recorder: recorder,
	}
}

func (g *agentGraph) Run(ctx context.Context, requirements string, knowledge string) agentGraphResult {
	result := agentGraphResult{
		Sections: []generatedSection{},
		Nodes:    make([]agentGraphNodeResult, 0, len(g.nodes)),
	}

	for _, node := range g.nodes {
		nodeResult := agentGraphNodeResult{Name: node.Name}
		output, err := runSubAgentWithRetry(ctx, node.Name, g.timeout, g.recorder, func(ctx context.Context) (string, error) {
			return node.Run(ctx, requirements, knowledge)
		})
		nodeResult.Output = output
		nodeResult.Err = err
		if err != nil {
			slog.Warn("sub-agent failed after retry, continuing without it",
				"agent", node.Name, "error", err)
			result.Nodes = append(result.Nodes, nodeResult)
			continue
		}

		parsed, parseErr := parseGeneratedSections(output)
		nodeResult.ParseErr = parseErr
		nodeResult.Sections = parsed
		if parseErr != nil || len(parsed) == 0 {
			nodeResult.SkippedEmpty = len(parsed) == 0
			slog.Warn("sub-agent produced unparseable output, skipping",
				"agent", node.Name, "parse_err", parseErr, "len", len(parsed))
			result.Nodes = append(result.Nodes, nodeResult)
			continue
		}
		result.Sections = append(result.Sections, parsed...)
		result.Nodes = append(result.Nodes, nodeResult)
	}

	return result
}
