ALTER TABLE fit_scores
    DROP COLUMN IF EXISTS verdict_profile_hash,
    DROP COLUMN IF EXISTS verdict_content_hash;

ALTER TABLE posting_embeddings DROP COLUMN IF EXISTS content_hash;

DROP INDEX IF EXISTS idx_postings_screen;
ALTER TABLE postings
    DROP COLUMN IF EXISTS screen_status,
    DROP COLUMN IF EXISTS screen_reason,
    DROP COLUMN IF EXISTS screen_profile_hash;
