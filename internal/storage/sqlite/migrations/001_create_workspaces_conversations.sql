CREATE TABLE workspaces (
    id          TEXT    NOT NULL PRIMARY KEY,
    name        TEXT    NOT NULL,
    root_path   TEXT    NOT NULL UNIQUE,
    pinned_at   INTEGER,
    archived_at INTEGER,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE conversations (
    id              TEXT    NOT NULL PRIMARY KEY,
    workspace_id    TEXT,
    title           TEXT    NOT NULL DEFAULT '',
    title_source    TEXT,
    model           TEXT    NOT NULL,
    last_message_at INTEGER,
    pinned_at       INTEGER,
    archived_at     INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,

    CHECK (
        title_source IS NULL
        OR title_source IN ('auto', 'manual')
    ),

    FOREIGN KEY (workspace_id)
        REFERENCES workspaces(id)
        ON DELETE SET NULL
);

CREATE INDEX idx_conversations_workspace_message
    ON conversations(workspace_id, last_message_at DESC);

CREATE INDEX idx_conversations_message
    ON conversations(last_message_at DESC);
