CREATE EXTENSION IF NOT EXISTS vector;

-- Seed list and discovery pipeline
CREATE TABLE companies (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    website_url  TEXT,
    ats_type     TEXT NOT NULL DEFAULT 'none'
                   CHECK (ats_type IN ('greenhouse','lever','ashby','workable','none')),
    ats_token    TEXT,  -- board token / site / subdomain used by the fetcher
    status       TEXT NOT NULL DEFAULT 'candidate'
                   CHECK (status IN ('candidate','confirmed','active','archived')),
    source       TEXT NOT NULL DEFAULT 'manual'
                   CHECK (source IN ('manual','built-in','referral','other')),
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Distinct tokens per ATS; candidates without a token do not conflict
CREATE UNIQUE INDEX uq_companies_ats_token
    ON companies (ats_type, ats_token) WHERE ats_token IS NOT NULL;

-- Recruiters, hiring managers, team leads (manual in MVP)
CREATE TABLE contacts (
    id           BIGSERIAL PRIMARY KEY,
    company_id   BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    title        TEXT,
    type         TEXT CHECK (type IN ('recruiter','hiring_manager','team_lead','referral','other')),
    email        TEXT,
    linkedin_url TEXT,
    source       TEXT NOT NULL DEFAULT 'manual',
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_contacts_company ON contacts(company_id);

-- Normalized job postings
CREATE TABLE postings (
    id                BIGSERIAL PRIMARY KEY,
    company_id        BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    source            TEXT NOT NULL CHECK (source IN ('greenhouse','lever','ashby','workable')),
    external_id       TEXT NOT NULL,          -- the ATS's own job id
    title             TEXT NOT NULL,
    location          TEXT,
    is_remote         BOOLEAN NOT NULL DEFAULT false,
    department        TEXT,
    employment_type   TEXT,
    salary_min        NUMERIC,
    salary_max        NUMERIC,
    salary_currency   TEXT,
    description       TEXT,
    apply_url         TEXT,
    source_url        TEXT,
    canonical_key     TEXT NOT NULL,          -- normalized company + title + location, for dedup
    content_hash      TEXT NOT NULL,          -- hash of normalized content, for change detection
    is_open           BOOLEAN NOT NULL DEFAULT true,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_updated_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, external_id)
);
CREATE INDEX idx_postings_company   ON postings(company_id);
CREATE INDEX idx_postings_open      ON postings(is_open);
CREATE INDEX idx_postings_canonical ON postings(canonical_key);

-- Embeddings, kept separate so re-embedding never rewrites the posting row.
-- Dimension 1024 matches the default Voyage AI model (voyage-3); if the
-- configured embedding model changes, this column must be migrated to match.
CREATE TABLE posting_embeddings (
    posting_id   BIGINT PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    embedding    vector(1024),
    model        TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_posting_embeddings_hnsw
    ON posting_embeddings USING hnsw (embedding vector_cosine_ops);

-- Match output that powers the digest
CREATE TABLE fit_scores (
    posting_id       BIGINT PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    semantic_score   REAL,       -- cosine similarity vs profile, 0..1
    llm_verdict      TEXT CHECK (llm_verdict IN ('pursue','stretch','skip')),
    llm_score        REAL,       -- optional numeric confidence
    llm_reasoning    TEXT,
    matched_role_tag TEXT CHECK (matched_role_tag IN (
                        'business-systems-analyst',
                        'implementation-specialist',
                        'technical-program-manager',
                        'technical-support-engineer',
                        'automation-engineer')),
    model            TEXT,
    computed_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_fit_scores_verdict  ON fit_scores(llm_verdict);
CREATE INDEX idx_fit_scores_semantic ON fit_scores(semantic_score DESC);

-- Application tracker
CREATE TABLE applications (
    id                  BIGSERIAL PRIMARY KEY,
    posting_id          BIGINT NOT NULL UNIQUE REFERENCES postings(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'identified'
                          CHECK (status IN ('identified','applied','interviewing',
                                            'closed_offer','closed_rejected','withdrawn')),
    applied_at          TIMESTAMPTZ,
    resume_variant      TEXT,     -- which role-specific resume/view was used
    used_cover_letter   BOOLEAN NOT NULL DEFAULT false,
    next_follow_up_date DATE,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_applications_status   ON applications(status);
CREATE INDEX idx_applications_followup ON applications(next_follow_up_date);

-- Communications log
CREATE TABLE correspondence (
    id               BIGSERIAL PRIMARY KEY,
    application_id   BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    contact_id       BIGINT REFERENCES contacts(id) ON DELETE SET NULL,
    direction        TEXT NOT NULL CHECK (direction IN ('inbound','outbound')),
    channel          TEXT CHECK (channel IN ('email','linkedin','phone','other')),
    summary          TEXT,
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    follow_up_needed BOOLEAN NOT NULL DEFAULT false,
    follow_up_date   DATE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_correspondence_app      ON correspondence(application_id);
CREATE INDEX idx_correspondence_followup ON correspondence(follow_up_date) WHERE follow_up_needed;
