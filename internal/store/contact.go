package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ContactTypes are the type values accepted by the contacts table.
var ContactTypes = []string{"recruiter", "hiring_manager", "team_lead", "referral", "other"}

// Contact is a row in the contacts table, joined with its company's name.
type Contact struct {
	ID          int64
	CompanyID   int64
	CompanyName string
	Name        string
	Title       string
	Type        string
	Email       string
	LinkedInURL string
	CreatedAt   time.Time
}

// NewContact holds the fields accepted when creating a contact via the CLI.
type NewContact struct {
	CompanyID int64
	Name      string
	Type      string
	Email     string
}

// CreateContact inserts a new contact under companyID.
func CreateContact(ctx context.Context, pool *pgxpool.Pool, in NewContact) (*Contact, error) {
	c := &Contact{}
	err := pool.QueryRow(ctx, `
		INSERT INTO contacts (company_id, name, type, email)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))
		RETURNING id, company_id, name, coalesce(title, ''), coalesce(type, ''),
		          coalesce(email, ''), coalesce(linkedin_url, ''), created_at
	`, in.CompanyID, in.Name, in.Type, in.Email).Scan(
		&c.ID, &c.CompanyID, &c.Name, &c.Title, &c.Type, &c.Email, &c.LinkedInURL, &c.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrCompanyNotFound
		}
		return nil, fmt.Errorf("creating contact: %w", err)
	}
	return c, nil
}

// ListContacts returns every contact, joined with its company's name,
// ordered by company then contact name.
func ListContacts(ctx context.Context, pool *pgxpool.Pool) ([]Contact, error) {
	rows, err := pool.Query(ctx, `
		SELECT ct.id, ct.company_id, c.name, ct.name, coalesce(ct.title, ''), coalesce(ct.type, ''),
		       coalesce(ct.email, ''), coalesce(ct.linkedin_url, ''), ct.created_at
		FROM contacts ct
		JOIN companies c ON c.id = ct.company_id
		ORDER BY c.name, ct.name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing contacts: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(
			&c.ID, &c.CompanyID, &c.CompanyName, &c.Name, &c.Title, &c.Type,
			&c.Email, &c.LinkedInURL, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning contact: %w", err)
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing contacts: %w", err)
	}
	return contacts, nil
}
