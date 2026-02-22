-- +goose Up

CREATE TABLE files (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    path          TEXT UNIQUE NOT NULL,
    file_hash     TEXT NOT NULL,
    version       INT DEFAULT 1,
    last_modified TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW(),
    status        TEXT DEFAULT 'pending'
);

CREATE TABLE chunks (
    id          TEXT PRIMARY KEY,
    file_id     UUID REFERENCES files(id),
    chunk_text  TEXT NOT NULL,
    position    TEXT,
    created_at  TIMESTAMP DEFAULT NOW()
);

CREATE TABLE embeddings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_id        TEXT REFERENCES chunks(id),
    vector_id       TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    created_at      TIMESTAMP DEFAULT NOW()
);

CREATE TABLE provenance_log (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    query_text     TEXT NOT NULL,
    used_chunks    JSONB,
    query_duration INT,
    created_at     TIMESTAMP DEFAULT NOW()
);

-- +goose Down

DROP TABLE provenance_log;
DROP TABLE embeddings;
DROP TABLE chunks;
DROP TABLE files;