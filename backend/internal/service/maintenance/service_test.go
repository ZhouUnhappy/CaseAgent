package maintenance

import "testing"

func TestClassifyDocumentVectorRow(t *testing.T) {
	t.Run("missing chunks with stored upload content is repairable", func(t *testing.T) {
		needsReprocess, blocked := classifyDocumentVectorRow(documentVectorRow{
			Source:     "upload",
			Content:    "# requirement",
			ChunkCount: 0,
		})

		if !needsReprocess || blocked {
			t.Fatalf("expected repairable document, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})

	t.Run("upload document without stored content is blocked", func(t *testing.T) {
		needsReprocess, blocked := classifyDocumentVectorRow(documentVectorRow{
			Source:                "upload",
			Content:               "   ",
			MissingEmbeddingCount: 1,
		})

		if !needsReprocess || !blocked {
			t.Fatalf("expected blocked document, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})

	t.Run("gdrive document without file id is blocked", func(t *testing.T) {
		needsReprocess, blocked := classifyDocumentVectorRow(documentVectorRow{
			Source:                   "gdrive",
			FileID:                   "",
			MismatchedEmbeddingCount: 2,
		})

		if !needsReprocess || !blocked {
			t.Fatalf("expected blocked gdrive document, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})

	t.Run("stale index with stored content is repairable", func(t *testing.T) {
		needsReprocess, blocked := classifyDocumentVectorRow(documentVectorRow{
			Source:          "upload",
			Content:         "# requirement",
			ChunkCount:      2,
			StaleIndexCount: 2,
		})

		if !needsReprocess || blocked {
			t.Fatalf("expected stale document to be repairable, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})
}

func TestClassifyKnowledgeVectorRow(t *testing.T) {
	t.Run("missing embedding with content is repairable", func(t *testing.T) {
		needsReprocess, blocked := classifyKnowledgeVectorRow(knowledgeVectorRow{
			Content:               "module spec",
			MissingEmbeddingCount: 1,
		})

		if !needsReprocess || blocked {
			t.Fatalf("expected repairable knowledge, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})

	t.Run("missing embedding without content is blocked", func(t *testing.T) {
		needsReprocess, blocked := classifyKnowledgeVectorRow(knowledgeVectorRow{
			Content:               " ",
			MissingEmbeddingCount: 1,
		})

		if !needsReprocess || !blocked {
			t.Fatalf("expected blocked knowledge, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})

	t.Run("stale index with content is repairable", func(t *testing.T) {
		needsReprocess, blocked := classifyKnowledgeVectorRow(knowledgeVectorRow{
			Content:         "module spec",
			StaleIndexCount: 1,
		})

		if !needsReprocess || blocked {
			t.Fatalf("expected stale knowledge to be repairable, got needsReprocess=%v blocked=%v", needsReprocess, blocked)
		}
	})
}
