CREATE TABLE IF NOT EXISTS washes (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    registration_number TEXT    NOT NULL,
    wash_type           TEXT    NOT NULL CHECK (wash_type IN ('basic', 'standard', 'premium')),
    status              TEXT    NOT NULL DEFAULT 'queued'
                                CHECK (status IN ('queued', 'in_progress', 'done', 'cancelled')),
    created_at          TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_washes_status ON washes (status);
