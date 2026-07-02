-- Screening state: which postings the evaluation pipeline may consider,
-- and why the excluded ones were excluded. screen_profile_hash records the
-- profile version the decision was made against, so a profile edit
-- re-screens automatically.
ALTER TABLE postings
    ADD COLUMN screen_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (screen_status IN ('pending','passed','excluded')),
    ADD COLUMN screen_reason TEXT,
    ADD COLUMN screen_profile_hash TEXT;
CREATE INDEX idx_postings_screen ON postings(is_open, screen_status);

-- Embedding freshness becomes self-describing: an embedding is current if
-- and only if its content_hash matches the posting's. Backfill assumes
-- existing embeddings reflect current content, which holds for any row the
-- current pipeline produced; wipe and re-sync instead if in doubt.
ALTER TABLE posting_embeddings ADD COLUMN content_hash TEXT;
UPDATE posting_embeddings pe
SET content_hash = p.content_hash
FROM postings p
WHERE p.id = pe.posting_id;

-- Verdict staleness: a verdict is trusted only while both hashes still
-- match. Semantic scores carry no hash because Stage 3 recomputes them
-- wholesale every run.
ALTER TABLE fit_scores
    ADD COLUMN verdict_profile_hash TEXT,
    ADD COLUMN verdict_content_hash TEXT;
