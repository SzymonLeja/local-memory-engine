-- +goose Up

CREATE TABLE jobs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id    UUID REFERENCES files(id),
    status     TEXT DEFAULT 'pending',   -- pending/running/done/error
    error      TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- +goose Down

DROP TABLE jobs;