package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// PostingDetail holds the fields needed to build an LLM fit-verdict prompt
// and to record which content version a verdict was computed against.
type PostingDetail struct {
	ID             int64
	CompanyName    string
	Title          string
	Location       string
	Description    string
	SalaryMin      *float64
	SalaryMax      *float64
	SalaryCurrency string
	ContentHash    string
}

// PostingDetails fetches prompt-relevant fields for the given posting IDs.
func PostingDetails(ctx context.Context, pool *pgxpool.Pool, postingIDs []int64) ([]PostingDetail, error) {
	if len(postingIDs) == 0 {
		return nil, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT p.id, c.name, p.title, coalesce(p.location, ''), coalesce(p.description, ''),
		       p.salary_min, p.salary_max, coalesce(p.salary_currency, ''), p.content_hash
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
			&d.SalaryMin, &d.SalaryMax, &d.SalaryCurrency, &d.ContentHash); err != nil {
			return nil, fmt.Errorf("scanning posting detail: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fetching posting details: %w", err)
	}
	return result, nil
}

// RefreshSemanticScores recomputes cosine similarity between
// profileEmbedding and every open, screened-in posting with a current
// embedding (Stage 3), in one statement. It only ever touches
// semantic_score and computed_at, never the verdict columns, so it is safe
// to run every sync regardless of what Stage 4 does afterward. It returns
// the number of postings scored.
func RefreshSemanticScores(ctx context.Context, pool *pgxpool.Pool, profileEmbedding []float32) (int64, error) {
	tag, err := pool.Exec(ctx, `
		INSERT INTO fit_scores (posting_id, semantic_score, computed_at)
		SELECT pe.posting_id, 1 - (pe.embedding <=> $1), now()
		FROM posting_embeddings pe
		JOIN postings p ON p.id = pe.posting_id
		WHERE p.is_open = true AND p.screen_status = 'passed'
		  AND pe.content_hash = p.content_hash
		ON CONFLICT (posting_id) DO UPDATE SET
			semantic_score = EXCLUDED.semantic_score,
			computed_at    = EXCLUDED.computed_at
	`, pgvector.NewVector(profileEmbedding))
	if err != nil {
		return 0, fmt.Errorf("refreshing semantic scores: %w", err)
	}
	return tag.RowsAffected(), nil
}

// VerdictCandidatePool returns, ranked by semantic score descending and
// capped at limit, open screened-in postings with a current embedding that
// have no verdict yet, or whose verdict is stale against profileHash or the
// posting's current content_hash. A posting whose LLM call previously
// failed has a row with llm_verdict NULL and thus stays in the pool
// automatically; a posting below the cutoff stays in the pool too and is
// picked up once it ranks into the top K. This single query is Stage 4's
// gate and its retry mechanism at once.
func VerdictCandidatePool(ctx context.Context, pool *pgxpool.Pool, profileHash string, minSemanticScore float64, limit int) ([]int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id
		FROM postings p
		JOIN posting_embeddings pe ON pe.posting_id = p.id AND pe.content_hash = p.content_hash
		LEFT JOIN fit_scores fs ON fs.posting_id = p.id
		WHERE p.is_open = true AND p.screen_status = 'passed'
		  AND (fs.posting_id IS NULL
		       OR fs.llm_verdict IS NULL
		       OR fs.verdict_profile_hash IS DISTINCT FROM $1
		       OR fs.verdict_content_hash IS DISTINCT FROM p.content_hash)
		  AND (fs.semantic_score IS NULL OR fs.semantic_score >= $2)
		ORDER BY fs.semantic_score DESC NULLS LAST
		LIMIT $3
	`, profileHash, minSemanticScore, limit)
	if err != nil {
		return nil, fmt.Errorf("selecting verdict candidate pool: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning verdict candidate: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("selecting verdict candidate pool: %w", err)
	}
	return ids, nil
}

// Verdict holds a Stage 4 LLM verdict outcome for one posting: either a
// successful verdict with both staleness hashes set, or a failure with
// only LLMReasoning set (and both hashes left nil), which keeps the
// posting in VerdictCandidatePool for automatic retry next run.
type Verdict struct {
	PostingID          int64
	LLMVerdict         *string // pursue | stretch | skip
	LLMScore           *float64
	LLMReasoning       *string
	MatchedRoleTag     *string
	Model              *string
	VerdictProfileHash *string
	VerdictContentHash *string
}

// UpsertVerdict stores or replaces the Stage 4 verdict fields for a
// posting. It never touches semantic_score: that column belongs solely to
// RefreshSemanticScores. In the normal case a fit_scores row already
// exists (Stage 3 creates one for every posting Stage 4 could possibly
// consider), so this is an UPDATE; the INSERT path exists only for the
// edge case of calling this before Stage 3 has run.
func UpsertVerdict(ctx context.Context, pool *pgxpool.Pool, v Verdict) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO fit_scores (
			posting_id, llm_verdict, llm_score, llm_reasoning, matched_role_tag,
			model, verdict_profile_hash, verdict_content_hash, computed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (posting_id) DO UPDATE SET
			llm_verdict          = EXCLUDED.llm_verdict,
			llm_score            = EXCLUDED.llm_score,
			llm_reasoning        = EXCLUDED.llm_reasoning,
			matched_role_tag     = EXCLUDED.matched_role_tag,
			model                = EXCLUDED.model,
			verdict_profile_hash = EXCLUDED.verdict_profile_hash,
			verdict_content_hash = EXCLUDED.verdict_content_hash,
			computed_at          = now()
	`, v.PostingID, v.LLMVerdict, v.LLMScore, v.LLMReasoning, v.MatchedRoleTag,
		v.Model, v.VerdictProfileHash, v.VerdictContentHash)
	if err != nil {
		return fmt.Errorf("upserting verdict for posting %d: %w", v.PostingID, err)
	}
	return nil
}
