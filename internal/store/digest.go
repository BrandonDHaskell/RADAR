package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DigestPosting is one row of the weekly digest: an open, screened-in
// posting with no application yet, alongside whatever fit score it has. A
// verdict is only surfaced while fresh; see VerdictStale.
type DigestPosting struct {
	ID             int64
	Title          string
	CompanyName    string
	Location       string
	IsRemote       bool
	SalaryMin      *float64
	SalaryMax      *float64
	SalaryCurrency string
	ApplyURL       string
	LLMVerdict     *string
	MatchedRoleTag *string
	LLMReasoning   *string
	SemanticScore  *float32
	// VerdictStale is true when this posting has a verdict in fit_scores
	// that no longer matches the current profile hash or the posting's
	// current content_hash. A stale verdict is treated as no verdict for
	// display and ranking purposes (the fields above are nil in that case)
	// but is still reported so the digest can tell the operator to re-sync.
	VerdictStale bool
}

// digestVerdictRank orders verdicts from most to least worth surfacing;
// a posting with no verdict yet (or a stale one) ranks below skip, not
// equal to it.
var digestVerdictRank = map[string]int{
	"":        0,
	"skip":    1,
	"stretch": 2,
	"pursue":  3,
}

// DigestPostings returns open, screened-in postings with no application
// yet, ranked by fit: a fresh LLM verdict first (pursue, then stretch,
// then skip), semantic score as the tiebreaker, and unscored or
// stale-verdict postings last. minVerdict ("", "pursue", "stretch", or
// "skip") excludes postings ranked below it; "" includes everything.
// profileHash is the current profile's hash, used to decide whether a
// stored verdict is still fresh.
func DigestPostings(ctx context.Context, pool *pgxpool.Pool, profileHash, minVerdict string, limit int) ([]DigestPosting, error) {
	rank, ok := digestVerdictRank[minVerdict]
	if !ok {
		return nil, fmt.Errorf("invalid min-verdict %q", minVerdict)
	}

	rows, err := pool.Query(ctx, `
		WITH ranked AS (
			SELECT p.id, p.title, c.name AS company_name, p.location, p.is_remote,
			       p.salary_min, p.salary_max, p.salary_currency, p.apply_url,
			       fs.semantic_score,
			       -- IS NOT DISTINCT FROM, not =, because pre-migration and
			       -- never-verdicted rows have NULL hashes: NULL = anything
			       -- is SQL NULL (breaks the boolean CASE logic below and
			       -- fails to scan into a Go bool), while NULL IS NOT
			       -- DISTINCT FROM a real hash is a clean false.
			       (fs.verdict_profile_hash IS NOT DISTINCT FROM $1
			        AND fs.verdict_content_hash IS NOT DISTINCT FROM p.content_hash) AS verdict_fresh,
			       (fs.llm_verdict IS NOT NULL
			        AND NOT (fs.verdict_profile_hash IS NOT DISTINCT FROM $1
			                 AND fs.verdict_content_hash IS NOT DISTINCT FROM p.content_hash)) AS verdict_stale,
			       fs.llm_verdict, fs.matched_role_tag, fs.llm_reasoning
			FROM postings p
			JOIN companies c ON c.id = p.company_id
			LEFT JOIN fit_scores fs ON fs.posting_id = p.id
			WHERE p.is_open = true AND p.screen_status != 'excluded'
			  AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.posting_id = p.id)
		)
		SELECT id, title, company_name, coalesce(location, ''), is_remote,
		       salary_min, salary_max, coalesce(salary_currency, ''), coalesce(apply_url, ''),
		       semantic_score, verdict_stale,
		       CASE WHEN verdict_fresh THEN llm_verdict ELSE NULL END,
		       CASE WHEN verdict_fresh THEN matched_role_tag ELSE NULL END,
		       CASE WHEN verdict_fresh THEN llm_reasoning ELSE NULL END,
		       CASE WHEN verdict_fresh THEN
		           CASE llm_verdict WHEN 'pursue' THEN 3 WHEN 'stretch' THEN 2 WHEN 'skip' THEN 1 ELSE 0 END
		       ELSE 0 END AS verdict_rank
		FROM ranked
		WHERE (CASE WHEN verdict_fresh THEN
		           CASE llm_verdict WHEN 'pursue' THEN 3 WHEN 'stretch' THEN 2 WHEN 'skip' THEN 1 ELSE 0 END
		       ELSE 0 END) >= $2
		ORDER BY verdict_rank DESC, semantic_score DESC NULLS LAST
		LIMIT $3
	`, profileHash, rank, limit)
	if err != nil {
		return nil, fmt.Errorf("querying digest postings: %w", err)
	}
	defer rows.Close()

	var result []DigestPosting
	for rows.Next() {
		var d DigestPosting
		var verdictRank int
		if err := rows.Scan(&d.ID, &d.Title, &d.CompanyName, &d.Location, &d.IsRemote,
			&d.SalaryMin, &d.SalaryMax, &d.SalaryCurrency, &d.ApplyURL,
			&d.SemanticScore, &d.VerdictStale,
			&d.LLMVerdict, &d.MatchedRoleTag, &d.LLMReasoning, &verdictRank); err != nil {
			return nil, fmt.Errorf("scanning digest posting: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("querying digest postings: %w", err)
	}
	return result, nil
}
