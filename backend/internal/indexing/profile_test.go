package indexing

import "testing"

func TestProfileFingerprintChangesWithEmbeddingModel(t *testing.T) {
	base := Profile{
		Name:                       ProfileName,
		EmbeddingProvider:          "fake",
		EmbeddingModel:             "model-a",
		Dimensions:                 4,
		DocumentChunkStrategy:      DocumentChunkStrategy,
		DocumentMaxChunkRunes:      DocumentMaxChunkRunes,
		KnowledgeEmbeddingStrategy: KnowledgeEmbeddingStrategy,
	}
	changed := base
	changed.EmbeddingModel = "model-b"

	if base.fingerprint() == changed.fingerprint() {
		t.Fatal("fingerprint should change when embedding model changes")
	}
}

func TestCurrentProfileWithoutLoadedConfig(t *testing.T) {
	profile := CurrentProfile()
	if profile.Name != ProfileName {
		t.Fatalf("profile name = %q, want %q", profile.Name, ProfileName)
	}
	if profile.Version == "" {
		t.Fatal("profile version should not be empty")
	}
}
