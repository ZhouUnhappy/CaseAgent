package indexing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"caseagent/internal/config"
)

const (
	ProfileName                = "caseagent-default"
	DocumentChunkStrategy      = "markdown_headers_v1"
	DocumentMaxChunkRunes      = 1200
	KnowledgeEmbeddingStrategy = "whole_entry_v1"
)

type Profile struct {
	Name                       string `json:"name"`
	Version                    string `json:"version"`
	EmbeddingProvider          string `json:"embedding_provider"`
	EmbeddingModel             string `json:"embedding_model"`
	Dimensions                 int    `json:"dimensions"`
	DocumentChunkStrategy      string `json:"document_chunk_strategy"`
	DocumentMaxChunkRunes      int    `json:"document_max_chunk_runes"`
	KnowledgeEmbeddingStrategy string `json:"knowledge_embedding_strategy"`
}

func CurrentProfile() Profile {
	cfg := config.Get()
	var embedding config.EmbeddingModelConfig
	if cfg != nil {
		embedding = cfg.Model.Embedding
	}

	provider := strings.ToLower(strings.TrimSpace(embedding.Provider))
	if provider == "" {
		provider = "unknown"
	}
	model := strings.TrimSpace(embedding.Model)
	if model == "" {
		model = "default"
	}

	profile := Profile{
		Name:                       ProfileName,
		EmbeddingProvider:          provider,
		EmbeddingModel:             model,
		Dimensions:                 embedding.Dimensions,
		DocumentChunkStrategy:      DocumentChunkStrategy,
		DocumentMaxChunkRunes:      DocumentMaxChunkRunes,
		KnowledgeEmbeddingStrategy: KnowledgeEmbeddingStrategy,
	}
	profile.Version = profile.fingerprint()
	return profile
}

func (p Profile) fingerprint() string {
	source := fmt.Sprintf("%s|%s|%s|%d|%s|%d|%s",
		p.Name,
		p.EmbeddingProvider,
		p.EmbeddingModel,
		p.Dimensions,
		p.DocumentChunkStrategy,
		p.DocumentMaxChunkRunes,
		p.KnowledgeEmbeddingStrategy,
	)
	sum := sha256.Sum256([]byte(source))
	return "v1-" + hex.EncodeToString(sum[:])[:12]
}
