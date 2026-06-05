package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	workflowservice "caseagent/internal/service/workflow"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type agentGraphInput struct {
	Requirements string `json:"requirements"`
	Knowledge    string `json:"knowledge"`
}

type agentGraphNode struct {
	Name        string
	Description string
	Agent       adk.Agent
	Enabled     func(agentGraphInput) bool
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
	Pruned       bool
	Duration     time.Duration
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
		input := agentGraphInput{Requirements: requirements, Knowledge: knowledge}
		if node.Enabled != nil && !node.Enabled(input) {
			nodeResult.Pruned = true
			result.Nodes = append(result.Nodes, nodeResult)
			continue
		}

		started := time.Now()
		output, err := runSubAgentWithRetry(ctx, node.Name, g.timeout, g.recorder, func(ctx context.Context) (string, error) {
			return runADKCaseAgent(ctx, node.Agent, input)
		},
			withAgentCallInputSummary(fmt.Sprintf("requirements_chars=%d knowledge_chars=%d", len(requirements), len(knowledge))),
			withAgentCallMetadata(map[string]any{
				"graph_node":        node.Name,
				"graph_node_type":   "sub_agent",
				"input_chars":       len(requirements) + len(knowledge),
				"requirements_size": len(requirements),
				"knowledge_size":    len(knowledge),
			}),
		)
		nodeResult.Duration = time.Since(started)
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

func runADKCaseAgent(ctx context.Context, agent adk.Agent, input agentGraphInput) (string, error) {
	if agent == nil {
		return "", fmt.Errorf("agent graph node has nil ADK agent")
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshal ADK agent input: %w", err)
	}
	iterator := agent.Run(ctx, &adk.AgentInput{
		Messages: []adk.Message{schema.UserMessage(string(raw))},
	})

	var output string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return "", event.Err
		}
		if event.Output == nil {
			continue
		}
		if text, ok := event.Output.CustomizedOutput.(string); ok && strings.TrimSpace(text) != "" {
			output = text
		}
		if event.Output.MessageOutput != nil {
			message, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				return "", err
			}
			if message != nil && strings.TrimSpace(message.Content) != "" {
				output = message.Content
			}
		}
	}
	if strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("ADK agent %q produced no output", agent.Name(ctx))
	}
	return output, nil
}

type caseGenerationADKAgent struct {
	name        string
	description string
	run         func(context.Context, string, string) (string, error)
}

func newCaseGenerationADKAgent(name string, description string, run func(context.Context, string, string) (string, error)) adk.Agent {
	return &caseGenerationADKAgent{
		name:        strings.TrimSpace(name),
		description: strings.TrimSpace(description),
		run:         run,
	}
}

func (a *caseGenerationADKAgent) Name(context.Context) string {
	return a.name
}

func (a *caseGenerationADKAgent) Description(context.Context) string {
	return a.description
}

func (a *caseGenerationADKAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()

		decoded, err := decodeADKCaseInput(input)
		if err != nil {
			generator.Send(&adk.AgentEvent{AgentName: a.name, Err: err})
			return
		}
		output, err := a.run(ctx, decoded.Requirements, decoded.Knowledge)
		if err != nil {
			generator.Send(&adk.AgentEvent{AgentName: a.name, Err: err})
			return
		}
		generator.Send(&adk.AgentEvent{
			AgentName: a.name,
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: schema.AssistantMessage(output, nil),
					Role:    schema.Assistant,
				},
				CustomizedOutput: output,
			},
		})
	}()
	return iterator
}

func decodeADKCaseInput(input *adk.AgentInput) (agentGraphInput, error) {
	if input == nil || len(input.Messages) == 0 || input.Messages[len(input.Messages)-1] == nil {
		return agentGraphInput{}, fmt.Errorf("ADK case agent input is empty")
	}
	var decoded agentGraphInput
	if err := json.Unmarshal([]byte(input.Messages[len(input.Messages)-1].Content), &decoded); err != nil {
		return agentGraphInput{}, fmt.Errorf("decode ADK case agent input: %w", err)
	}
	if strings.TrimSpace(decoded.Requirements) == "" {
		return agentGraphInput{}, fmt.Errorf("ADK case agent requirements are empty")
	}
	return decoded, nil
}
