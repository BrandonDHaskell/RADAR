-- 'active' was never reachable from the CLI and was excluded by sync.
-- Company status now only answers whether sync should consider the board.
UPDATE companies SET status = 'confirmed', updated_at = now() WHERE status = 'active';

ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_status_check;
ALTER TABLE companies ADD CONSTRAINT companies_status_check
    CHECK (status IN ('candidate','confirmed','archived'));
