package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DigestPosting is one row of the weekly digest: an open posting with no
// application yet, alongside whatever fit score it has.
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
}

// digestVerdictRank orders verdicts from most to least worth surfacing;
// a posting with no verdict yet ranks below skip, not equal to it.
var digestVerdictRank = map[string]int{
	"":        0,
	"skip":    1,
	"stretch": 2,
	"pursue":  3,
}

// DigestPostings returns open postings with no application yet, ranked by
// fit: LLM verdict first (pursue, then stretch, then skip, then unscored),
// semantic score as the tiebreaker. minVerdict ("", "pursue", "stretch", or
// "skip") excludes postings ranked below it; "" includes everything.
func DigestPostings(ctx context.Context, pool *pgxpool.Pool, minVerdict string, limit int) ([]DigestPosting, error) {
	rank, ok := digestVerdictRank[minVerdict]
	if !ok {
		return nil, fmt.Errorf("invalid min-verdict %q", minVerdict)
	}

	rows, err := pool.Query(ctx, `
		WITH ranked AS (
			SELECT p.id, p.title, c.name AS company_name, p.location, p.is_remote,
			       p.salary_min, p.salary_max, p.salary_currency, p.apply_url,
			       fs.llm_verdict, fs.matched_role_tag, fs.llm_reasoning, fs.semantic_score,
			       CASE fs.llm_verdict
			           WHEN 'pursue'  THEN 3
			           WHEN 'stretch' THEN 2
			           WHEN 'skip'    THEN 1
			           ELSE 0
			       END AS verdict_rank
			FROM postings p
			JOIN companies c ON c.id = p.company_id
			LEFT JOIN fit_scores fs ON fs.posting_id = p.id
			WHERE p.is_open = true
			  AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.posting_id = p.id)
		)
		SELECT id, title, company_name, coalesce(location, ''), is_remote,
		       salary_min, salary_max, coalesce(salary_currency, ''), coalesce(apply_url, ''),
		       llm_verdict, matched_role_tag, llm_reasoning, semantic_score
		FROM ranked
		WHERE verdict_rank >= $1
		ORDER BY verdict_rank DESC, semantic_score DESC NULLS LAST
		LIMIT $2
	`, rank, limit)
	if err != nil {
		return nil, fmt.Errorf("querying digest postings: %w", err)
	}
	defer rows.Close()

	var result []DigestPosting
	for rows.Next() {
		var d DigestPosting
		if err := rows.Scan(&d.ID, &d.Title, &d.CompanyName, &d.Location, &d.IsRemote,
			&d.SalaryMin, &d.SalaryMax, &d.SalaryCurrency, &d.ApplyURL,
			&d.LLMVerdict, &d.MatchedRoleTag, &d.LLMReasoning, &d.SemanticScore); err != nil {
			return nil, fmt.Errorf("scanning digest posting: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("querying digest postings: %w", err)
	}
	return result, nil
}
