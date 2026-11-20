CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    username   TEXT NOT NULL,
    role       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL
);
CREATE INDEX sessions_last_seen ON sessions(last_seen);
