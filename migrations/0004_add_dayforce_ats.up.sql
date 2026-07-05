ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_ats_type_check;
ALTER TABLE companies ADD CONSTRAINT companies_ats_type_check
    CHECK (ats_type IN ('greenhouse','lever','ashby','workable','dayforce','none'));

ALTER TABLE postings DROP CONSTRAINT IF EXISTS postings_source_check;
ALTER TABLE postings ADD CONSTRAINT postings_source_check
    CHECK (source IN ('greenhouse','lever','ashby','workable','dayforce'));
