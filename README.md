# Local Memory Engine (LME) + Open WebUI Functions

A minimal **RAG** service for a local Markdown “vault”, built on:
- **PostgreSQL** (file metadata, chunks, jobs, provenance)
- **Qdrant** (vectors + payload)
- **Ollama** (embeddings)

This repo also contains an **Open WebUI function/filter** (`openui-functions/openui-functions.py`) that:
1) on a user prompt, fetches context from LME (`/query`) and injects it into the conversation,
2) after the assistant replies, appends the Q/A back into the vault via LME (`PATCH/POST /ingest`).

## How it works (high level)

1. **Ingest**: LME scans the vault directory (default `./vault`) and indexes `.md` files.
2. Each file is split into chunks (~512 tokens, ~50 token overlap).
3. For each chunk, LME generates embeddings via **Ollama** and stores them in **Qdrant**.
4. **Query**: for a user query, LME embeds the query, runs a `search` in Qdrant, then enriches results with metadata and text from Postgres.
5. **Provenance**: each query can be stored in `provenance_log` (query + chunks used + time).

## Requirements

- Docker + Docker Compose (recommended)
- or locally: Go 1.25+, PostgreSQL 16+, Qdrant, Ollama

## Quick start (Docker Compose)

This repo includes a ready-to-run `docker-compose.yml`:

```bash
docker compose up --build
```

LME will be available at:
- `http://localhost:8080`

The vault (Markdown) is mounted as a volume:
- host: `./vault`
- container: `/vault`

### First indexing run

Index the whole vault:

```bash
curl -X POST http://localhost:8080/ingest   -H 'Content-Type: application/json'   -H 'X-API-Key: change-me'   -d '{"path":"."}'
```

Check job status:

```bash
curl http://localhost:8080/status/<job_id>   -H 'X-API-Key: change-me'
```

## Configuration (.env / env vars)

LME reads `.env` (if present) and/or environment variables.

**Required:**
- `POSTGRES_DSN` – e.g. `postgres://lme:lme@localhost:5432/lme?sslmode=disable`
- `QDRANT_URL` – e.g. `http://localhost:6333`
- `OLLAMA_URL` – e.g. `http://localhost:11434`

**Optional (with sensible defaults):**
- `LISTEN_ADDR` (default `:8080`)
- `EMBEDDING_MODEL` (default `nomic-embed-text`)
- `QDRANT_COLLECTION` (default `lme`)
- `VAULT_ROOT` (default `./vault`)
- `WATCH_PATH` (default `.`) – relative directory inside the vault to watch
- `ALLOWED_ORIGINS` (default `http://localhost`) – CSV, e.g. `http://localhost:3000,http://127.0.0.1:3000`
- `API_KEY` – if set, all endpoints except `/health` require the `X-API-Key` header

A `.env.example` is included.

## API (HTTP)

### Health

`GET /health` → `200 {"status":"ok"}`

### Ingest (indexing)

`POST /ingest`

Modes:
1) **Ingest a path** (a folder inside the vault):

```bash
curl -X POST http://localhost:8080/ingest   -H 'Content-Type: application/json'   -H 'X-API-Key: <key>'   -d '{"path":"api-notes"}'
```

2) **Direct ingest** (write a file + index it):

```bash
curl -X POST http://localhost:8080/ingest   -H 'Content-Type: application/json'   -H 'X-API-Key: <key>'   -d '{"filename":"agent-note","path":"api-notes","content":"# Hello\n\ncontent...","format":"md"}'
```

3) **Multipart upload** (`file` field, `.md` only):

```bash
curl -X POST http://localhost:8080/ingest   -H 'X-API-Key: <key>'   -F 'file=@./vault/api-notes/agent-note.md'   -F 'filename=agent-note'   -F 'path=api-notes'
```

Example response:

```json
{ "job_id": "...", "new_files": 1, "updated_files": 0, "skipped": 0 }
```

### Patch / edit a file (append or overwrite)

`PATCH /ingest`

- `append` – appends to the end of the file
- `content` – overwrites the entire content

```bash
curl -X PATCH http://localhost:8080/ingest   -H 'Content-Type: application/json'   -H 'X-API-Key: <key>'   -d '{"filename":"agent-note","path":"api-notes","append":"\n## addendum\n..."}'
```

If the file does not exist → `404`.  
If the filename matches multiple files (and `path` is omitted) → `300` with a list of matches.

### Query (RAG)

`POST /query`

```bash
curl -X POST http://localhost:8080/query   -H 'Content-Type: application/json'   -H 'X-API-Key: <key>'   -d '{"q":"what do we know about X?","top_k":5}'
```

Response:

```json
{
  "query_id": "...",
  "duration_ms": 12,
  "results": [
    {
      "chunk_text": "...",
      "file_path": "api-notes/agent-note.md",
      "position": "paragraph 3",
      "score": 0.78
    }
  ]
}
```

### Provenance

`GET /provenance/{id}` – returns a record from `provenance_log`.

### Job status

`GET /status/{job_id}` – returns status (`pending/running/done/error`) and an error (if any).

### Download a file

`GET /file/{filename}?format=md&path=api-notes`

- `format` – currently only `md`
- `path` – optionally narrow to a specific directory; without it the endpoint tries to find the best match, and if there are multiple matches it returns `300`.

## Watcher (auto re-index)

If `WATCH_PATH` is not empty, LME starts an `fsnotify` watcher and re-indexes the file’s directory on changes.
- Ignores hidden paths (`/.`)
- Only reacts to `.md`
- Debounce ~500ms

In `docker-compose.yml` the watcher is enabled by default (`WATCH_PATH: "."`).

## Open WebUI integration (Function/Filter)

File: `openui-functions/openui-functions.py`

What it does:
- **inlet**: before calling the LLM, queries LME for top-k results and adds them as a `system` message.
- **outlet**: after the assistant response, appends Q/A into a vault file:
  - first tries `PATCH /ingest` with `append`
  - if it gets `404` → creates a new file via `POST /ingest`

### Filter configuration (Valves)

The filter exposes parameters:
- `lme_url` (default `http://host.docker.internal:8080`)
- `lme_key` (default `demo123`)
- `topk` (default `3`)

Set them to match your deployment (the API key must match `API_KEY` in LME).

> Note: the filter has a small mismatch — `/query` results do not include a `filename` field (they include `file_path`). If you want to fetch the “full top1 file”, it’s best to call `GET /file/{filename}` using the basename (without `.md`) and `path`, or update the filter to parse `file_path` (e.g., extract basename without extension and directory as `path`).

## Development (local, without Docker)

1. Start Postgres, Qdrant and Ollama.
2. Copy `.env.example` → `.env` and fill it in.
3. Run:

```bash
go run ./cmd/lme
```

Migrations run automatically on startup (goose).

## Repository structure

- `cmd/lme` – HTTP API entrypoint
- `internal/ingest` – vault walk, chunking, watcher, file CRUD
- `internal/query` – query embedding + Qdrant search + Postgres enrich
- `internal/vector` – Qdrant client
- `internal/embeddings` – Ollama client
- `internal/provenance` – query logging
- `internal/jobs` – job statuses
- `migrations/` – Postgres schema
- `vault/` – example vault (Markdown)
- `openui-functions/` – Open WebUI filter

## Troubleshooting

- **401 unauthorized**: verify the `X-API-Key` header and `API_KEY` config.
- **Qdrant collection**: on startup LME creates/ensures the collection exists (default `lme`) with dim `768`.
- **Ollama model**: make sure the embedding model is available in Ollama (in compose this is handled by `scripts/ollama-init.sh`).
- **No results**: run ingest on the directory containing `.md` files and check the LME container logs.

---

