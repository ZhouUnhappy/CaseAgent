package models

// All models
var models = []interface{}{
	&Tenant{},
	&Project{},
	&Document{},
	&DocumentChunk{},
	&KnowledgeBase{},
	&TestCase{},
	&TestCaseFeedback{},
	&CaseGenerationTask{},
	&BackgroundJob{},
	&WorkflowRun{},
	&WorkflowStep{},
	&AgentRun{},
	&ModelCall{},
	&RetrievalRun{},
	&Artifact{},
	&KnowledgeUpdateSuggestionGroup{},
	&KnowledgeUpdateSuggestionOccurrence{},
}
