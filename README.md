# RADAR

**R**ole **A**ggregation, **D**iscovery, **A**ssessment, and **R**anking

A single-user job-search engine. RADAR pulls postings from public ATS job-board APIs, ranks them against your verified profile using semantic embeddings and an LLM fit verdict, and drives a weekly application routine through a CLI and a Markdown or HTML digest. It runs locally as a background service; there is no web UI and no hosted component.

## Why

Running a targeted job search across many companies means visiting many separate career pages. RADAR consolidates that: it fetches postings from a curated set of companies, scores each one honestly against your actual background, and surfaces only the postings worth your time.

Honesty is a hard requirement, not a nice-to-have. The LLM fit verdict is instructed to assess fit against your verified profile only, never to invent or inflate qualifications, and to prefer a defensible `skip` over an optimistic `pursue`.

## Status

RADAR is built in phases; each phase is independently useful. Current state:

| Phase | What | Status |
| --- | --- | --- |
| 0 | Scaffold: CLI skeleton, config, Postgres, migrations | Done |
| 1 | Schema and company CRUD (`company add\|list\|confirm\|archive`) | Done |
| 2 | Greenhouse adapter, `sync` (fetch, dedup, expire) | Done |
| 3 | Profile loading and embeddings (Voyage AI) | Done |
| 4 | Fit scoring: semantic similarity + LLM verdict (Claude) | Done |
| 4.5 | Consolidated matching pipeline: Stage 0 screening, top-K verdict gating, profile hashing | Done |
| 5 | Digest: Markdown and HTML | Done |
| 6 | Lever, Ashby, Workable adapters | Done |
| 7 | Application tracking (`apply`, `log`, `followups`, `close`, `contact`) | Done |
| 8 | Background service (`serve`) | Not started (stub only) |
| 9 | Discovery (`discover`, Built In SF, best-effort) | Not started (stub only) |

Run `radar --help` for the full command list; commands not yet implemented say so when invoked.

## Architecture

Single Go binary, two modes: one-shot CLI commands today, plus a future `serve` daemon that runs the same operations on a schedule. Domain logic lives in `internal/` and has no dependency on the CLI, so a future web layer can reuse it without a rewrite.

```
cmd/radar/       CLI (Cobra), thin adapter over internal/
internal/
  config/        config.yaml + env secrets loading
  store/         Postgres access (pgx), migrations, queries (including applications, correspondence, contacts)
  ingest/        Fetcher interface + one adapter per ATS (Greenhouse, Lever, Ashby, Workable)
  normalize/     canonical key + content hash for dedup and change detection
  dedup/         upsert + expiry orchestration
  embed/         embedding provider interface + Voyage AI implementation
  llm/           LLM provider interface + Claude implementation
  match/         profile loading, the staged evaluation pipeline, LLM prompts
  digest/        Markdown/HTML rendering
  discover/      company discovery (Phase 9)
  schedule/      cron wiring for serve mode (Phase 8)
migrations/      SQL migrations, embedded into the binary
templates/       digest templates, embedded into the binary
```

Postgres 16+ with `pgvector` is the only external dependency besides the embedding and LLM APIs. `docker-compose.yml` runs it locally.

### Evaluation pipeline

`radar sync` fetches per company, then runs one corpus-wide evaluation pass over every confirmed board's postings together, so the profile is embedded once per run and ranking is global rather than per company:

1. **Screen** (free, deterministic): title exclusions (`preferences.title_exclusions` in `profile.json`, case-insensitive whole-phrase match) and a location filter, both biased toward false positives passing through rather than hiding a real match. Excluded postings never reach embedding or the LLM. Review what got excluded with `radar excluded`; there is no un-exclude command, correct a wrong exclusion by editing `profile.json`.
2. **Embed**: any screened-in posting whose embedding is missing or whose content changed since it was last embedded.
3. **Semantic score**: cosine similarity against the profile, recomputed for every screened-in posting on every run.
4. **Verdict**: only the top `match.llm_top_k` postings by semantic score (default 40) get an LLM call. A failed or stale verdict (profile edited, or posting content changed) stays in this pool and is retried automatically on the next sync.

Editing `profile.json` invalidates screening and verdicts for the whole corpus on the next sync (cheap: only the shortlist gets re-verdicted) but never touches existing embeddings.

## Quickstart

Prerequisites: Go 1.25+, Docker, a [Voyage AI](https://www.voyageai.com/) API key, and an [Anthropic](https://console.anthropic.com/) API key.

```sh
# 1. Secrets and local Postgres
cp .env.example .env               # fill in VOYAGE_API_KEY and ANTHROPIC_API_KEY
docker compose up -d               # starts Postgres + pgvector on localhost:5432

# 2. Your profile
mkdir -p ~/.config/radar
cp profile.example.json ~/.config/radar/profile.json   # then edit it: your real, verified background,
                                                         # including preferences.title_exclusions (see Evaluation pipeline below)

# 3. Config (optional; sane defaults apply without this)
cp config.example.yaml ~/.config/radar/config.yaml

# 4. Build
go build -o radar ./cmd/radar

# 5. Add a company and pull its postings
set -a; source .env; set +a
./radar company add --name "Acme" --ats greenhouse --token acme-board-token
./radar company list                       # note the id printed for Acme, e.g. 1
./radar company confirm <id>
./radar sync

# 6. Review the results
./radar digest --format md --limit 10      # config.example.yaml sets digest.out_path, so this writes to ~/radar-digest.md
cat ~/radar-digest.md                      # or pass --out /dev/stdout to print directly
```

`sync` fetches, dedupes, embeds new or changed postings, and scores them (semantic similarity plus an LLM verdict) in one pass. `digest` reads what `sync` already scored; it makes no API calls of its own.

Secrets (`DATABASE_URL`, `VOYAGE_API_KEY`, `ANTHROPIC_API_KEY`) come from the environment only, never from `config.yaml`. `~/.config/radar/profile.json` and `~/.config/radar/config.yaml` are personal and are not part of this repo.

## CLI reference

```
radar company add|list|confirm|archive   manage the seed list of companies
radar sync                               fetch, dedupe, then screen/embed/score/verdict the whole corpus
radar digest                             render the ranked digest (--format md|html, --limit, --min-verdict, --out)
radar excluded                           list recently excluded postings, for the weekly false-negative review (--limit)
radar apply <posting_id>                 mark a posting applied (--resume-variant, --cover-letter, --follow-up)
radar log <application_id>               log a correspondence entry (--direction, --channel, --summary, --contact, --follow-up[-date])
radar followups                          list applications due for follow-up today (--stale N also surfaces no-activity applications)
radar close <application_id>             close out an application (--status closed_offer|closed_rejected|withdrawn)
radar contact add|list                   manage contacts (--company, --name, --type, --email)
```

Everything else (`discover`, `serve`) is scaffolded but not yet implemented; see Status above. Run `radar <command> --help` for flags.

## Development

```sh
docker compose up -d
set -a; source .env; set +a
go test ./...          # integration tests run against the live Postgres above; they skip if DATABASE_URL is unset
gofmt -l .
go vet ./...
```

Migrations live in `migrations/*.sql` and are embedded into the binary; `radar` applies any pending migration automatically on startup, so there is no separate migrate step.

Integration tests assume a mostly-empty dev database and clean up their own rows via `t.Cleanup`, but a few (`DigestPostings`, which intentionally ranks across every company) are not isolated from other data already sitting in Postgres. If you've been manually testing with `radar company add` / `radar sync` against the same database, delete those companies (cascades to their postings) before running `go test ./...`, or point `DATABASE_URL` at a fresh `docker compose down -v && docker compose up -d`.
