package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// SemanticScore is a computed similarity between the profile and one posting.
type SemanticScore struct {
	PostingID int64
	Score     float32 // cosine similarity: 1 - cosine distance
}

// SemanticScores computes cosine similarity between profileEmbedding and
// each posting in postingIDs that has a stored embedding. A posting with no
// embedding yet is simply absent from the result.
func SemanticScores(ctx context.Context, pool *pgxpool.Pool, profileEmbedding []float32, postingIDs []int64) ([]SemanticScore, error) {
	if len(postingIDs) == 0 {
		return nil, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT posting_id, 1 - (embedding <=> $1) AS semantic_score
		FROM posting_embeddings
		WHERE posting_id = ANY($2::bigint[])
	`, pgvector.NewVector(profileEmbedding), postingIDs)
	if err != nil {
		return nil, fmt.Errorf("computing semantic scores: %w", err)
	}
	defer rows.Close()

	var result []SemanticScore
	for rows.Next() {
		var s SemanticScore
		var score float64
		if err := rows.Scan(&s.PostingID, &score); err != nil {
			return nil, fmt.Errorf("scanning semantic score: %w", err)
		}
		s.Score = float32(score)
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("computing semantic scores: %w", err)
	}
	return result, nil
}

// PostingDetail holds the fields needed to build an LLM fit-verdict prompt.
type PostingDetail struct {
	ID             int64
	CompanyName    string
	Title          string
	Location       string
	Description    string
	SalaryMin      *float64
	SalaryMax      *float64
	SalaryCurrency string
}

// PostingDetails fetches prompt-relevant fields for the given posting IDs.
func PostingDetails(ctx context.Context, pool *pgxpool.Pool, postingIDs []int64) ([]PostingDetail, error) {
	if len(postingIDs) == 0 {
		return nil, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT p.id, c.name, p.title, coalesce(p.location, ''), coalesce(p.description, ''),
		       p.salary_min, p.salary_max, coalesce(p.salary_currency, '')
		FROM postings p
		JOIN companies c ON c.id = p.company_id
		WHERE p.id = ANY($1::bigint[])
	`, postingIDs)
	if err != nil {
		return nil, fmt.Errorf("fetching posting details: %w", err)
	}
	defer rows.Close()

	var result []PostingDetail
	for rows.Next() {
		var d PostingDetail
		if err := rows.Scan(&d.ID, &d.CompanyName, &d.Title, &d.Location, &d.Description,
			&d.SalaryMin, &d.SalaryMax, &d.SalaryCurrency); err != nil {
			return nil, fmt.Errorf("scanning posting detail: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fetching posting details: %w", err)
	}
	return result, nil
}

// PostingsMissingFitScore returns open posting IDs for companyID/source that
// have no fit_scores row yet. ScorePostings only re-queues postings whose
// content changed, so a posting whose scoring previously failed (LLM error,
// rate limit) would otherwise never be retried.
func PostingsMissingFitScore(ctx context.Context, pool *pgxpool.Pool, companyID int64, source string) ([]int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id
		FROM postings p
		LEFT JOIN fit_scores fs ON fs.posting_id = p.id
		WHERE p.company_id = $1 AND p.source = $2 AND p.is_open = true AND fs.posting_id IS NULL
	`, companyID, source)
	if err != nil {
		return nil, fmt.Errorf("finding postings missing fit scores for company %d/%s: %w", companyID, source, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning posting id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finding postings missing fit scores for company %d/%s: %w", companyID, source, err)
	}
	return ids, nil
}

// FitScore holds a computed, or partially computed, fit assessment for one
// posting. LLM fields are nil when only the semantic score was available
// (see ScorePostings' defensive fallback).
type FitScore struct {
	PostingID      int64
	SemanticScore  *float32
	LLMVerdict     *string // pursue | stretch | skip
	LLMScore       *float64
	LLMReasoning   *string
	MatchedRoleTag *string
	Model          *string
}

// UpsertFitScore stores or replaces the fit score for a posting.
func UpsertFitScore(ctx context.Context, pool *pgxpool.Pool, fs FitScore) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO fit_scores (posting_id, semantic_score, llm_verdict, llm_score, llm_reasoning, matched_role_tag, model, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (posting_id) DO UPDATE SET
			semantic_score   = EXCLUDED.semantic_score,
			llm_verdict      = EXCLUDED.llm_verdict,
			llm_score        = EXCLUDED.llm_score,
			llm_reasoning    = EXCLUDED.llm_reasoning,
			matched_role_tag = EXCLUDED.matched_role_tag,
			model            = EXCLUDED.model,
			computed_at      = now()
	`, fs.PostingID, fs.SemanticScore, fs.LLMVerdict, fs.LLMScore, fs.LLMReasoning, fs.MatchedRoleTag, fs.Model)
	if err != nil {
		return fmt.Errorf("upserting fit score for posting %d: %w", fs.PostingID, err)
	}
	return nil
}
