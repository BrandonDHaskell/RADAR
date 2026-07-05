package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidATSTypes are the ats_type values accepted by the companies table.
var ValidATSTypes = []string{"greenhouse", "lever", "ashby", "workable", "dayforce", "none"}

// CompanyStatuses are the status values a company can hold. Status answers
// exactly one question: should sync consider this board (confirmed) or not
// (candidate, archived). Engagement with a company lives in applications,
// not here.
const (
	CompanyStatusCandidate = "candidate"
	CompanyStatusConfirmed = "confirmed"
	CompanyStatusArchived  = "archived"
)

// ErrDuplicateCompany is returned when a company with the same ats_type and
// ats_token already exists.
var ErrDuplicateCompany = errors.New("a company with this ATS type and token already exists")

// ErrCompanyNotFound is returned when a company id does not exist.
var ErrCompanyNotFound = errors.New("company not found")

// Company is a row in the companies table.
type Company struct {
	ID         int64
	Name       string
	WebsiteURL string
	ATSType    string
	ATSToken   string
	Status     string
	Source     string
	Notes      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewCompany holds the fields accepted when creating a company via the CLI.
// New companies always start in the "candidate" status and "manual" source;
// they are promoted by ConfirmCompany once the operator reviews them.
type NewCompany struct {
	Name       string
	WebsiteURL string
	ATSType    string
	ATSToken   string
	Notes      string
}

// CreateCompany inserts a new candidate company.
func CreateCompany(ctx context.Context, pool *pgxpool.Pool, in NewCompany) (*Company, error) {
	c := &Company{}
	err := pool.QueryRow(ctx, `
		INSERT INTO companies (name, website_url, ats_type, ats_token, notes)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), NULLIF($5, ''))
		RETURNING id, name, coalesce(website_url, ''), ats_type, coalesce(ats_token, ''),
		          status, source, coalesce(notes, ''), created_at, updated_at
	`, in.Name, in.WebsiteURL, in.ATSType, in.ATSToken, in.Notes).Scan(
		&c.ID, &c.Name, &c.WebsiteURL, &c.ATSType, &c.ATSToken,
		&c.Status, &c.Source, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateCompany
		}
		return nil, fmt.Errorf("inserting company: %w", err)
	}
	return c, nil
}

// ListCompanies returns companies, optionally filtered by status. An empty
// status returns every company, ordered by name.
func ListCompanies(ctx context.Context, pool *pgxpool.Pool, status string) ([]Company, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, coalesce(website_url, ''), ats_type, coalesce(ats_token, ''),
		       status, source, coalesce(notes, ''), created_at, updated_at
		FROM companies
		WHERE $1 = '' OR status = $1
		ORDER BY name
	`, status)
	if err != nil {
		return nil, fmt.Errorf("listing companies: %w", err)
	}
	defer rows.Close()

	var companies []Company
	for rows.Next() {
		var c Company
		if err := rows.Scan(
			&c.ID, &c.Name, &c.WebsiteURL, &c.ATSType, &c.ATSToken,
			&c.Status, &c.Source, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning company: %w", err)
		}
		companies = append(companies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing companies: %w", err)
	}
	return companies, nil
}

// SetCompanyStatus updates a company's status (used by confirm and archive)
// and returns the updated row.
func SetCompanyStatus(ctx context.Context, pool *pgxpool.Pool, id int64, status string) (*Company, error) {
	c := &Company{}
	err := pool.QueryRow(ctx, `
		UPDATE companies
		SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, name, coalesce(website_url, ''), ats_type, coalesce(ats_token, ''),
		          status, source, coalesce(notes, ''), created_at, updated_at
	`, id, status).Scan(
		&c.ID, &c.Name, &c.WebsiteURL, &c.ATSType, &c.ATSToken,
		&c.Status, &c.Source, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCompanyNotFound
		}
		return nil, fmt.Errorf("updating company %d: %w", id, err)
	}
	return c, nil
}
