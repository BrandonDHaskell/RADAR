// Package embed defines the embedding provider interface used to vectorize
// the operator's profile and job postings, plus a Voyage AI implementation.
// The pipeline is written against the interface so a different provider
// (hosted or local) can be swapped in without touching callers.
package embed

import "context"

// InputType hints whether texts are search queries or the documents being
// searched over. Voyage produces better-matched vectors when this is set
// correctly: the operator's profile is a query, postings are documents.
type InputType string

const (
	InputTypeQuery    InputType = "query"
	InputTypeDocument InputType = "document"
)

// Provider embeds text into fixed-dimension vectors.
type Provider interface {
	// Embed returns one embedding vector per entry in texts, in the same
	// order. All returned vectors have the same length, equal to Dimension.
	Embed(ctx context.Context, texts []string, inputType InputType) ([][]float32, error)
	// Dimension returns the length of vectors this provider returns. It
	// must match the posting_embeddings.embedding column's declared
	// dimension; callers should fail fast on mismatch rather than let
	// Postgres reject the insert with a less clear error.
	Dimension() int
}
