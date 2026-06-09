-- Initial schema for the local ticket tracker.

CREATE TABLE projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key         TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    repo_path   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE tickets (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id   INTEGER NOT NULL,
    number       INTEGER NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL,
    status       TEXT NOT NULL,
    priority     TEXT NOT NULL,
    labels       TEXT NOT NULL DEFAULT '[]',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    completed_at TEXT,
    UNIQUE(project_id, number),
    FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_tickets_project_status ON tickets(project_id, status);

CREATE TABLE subtasks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id  INTEGER NOT NULL,
    title      TEXT NOT NULL,
    is_done    INTEGER NOT NULL DEFAULT 0,
    position   INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(ticket_id) REFERENCES tickets(id) ON DELETE CASCADE
);

CREATE TABLE comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id  INTEGER NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(ticket_id) REFERENCES tickets(id) ON DELETE CASCADE
);

CREATE TABLE attachments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id     INTEGER NOT NULL,
    file_name     TEXT NOT NULL,
    original_path TEXT,
    stored_path   TEXT NOT NULL,
    mime_type     TEXT,
    size_bytes    INTEGER NOT NULL,
    created_at    TEXT NOT NULL,
    FOREIGN KEY(ticket_id) REFERENCES tickets(id) ON DELETE CASCADE
);

CREATE TABLE activity_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id      INTEGER NOT NULL,
    event_type     TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1,
    payload        TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    FOREIGN KEY(ticket_id) REFERENCES tickets(id) ON DELETE CASCADE
);

CREATE INDEX idx_activity_ticket ON activity_events(ticket_id, id);

-- Full-text search over tickets (external-content FTS5: the index mirrors the
-- tickets table and is kept in sync by the triggers below).
CREATE VIRTUAL TABLE ticket_search USING fts5(
    title,
    description,
    labels,
    content='tickets',
    content_rowid='id'
);

CREATE TRIGGER tickets_ai AFTER INSERT ON tickets BEGIN
    INSERT INTO ticket_search(rowid, title, description, labels)
    VALUES (new.id, new.title, new.description, new.labels);
END;

CREATE TRIGGER tickets_ad AFTER DELETE ON tickets BEGIN
    INSERT INTO ticket_search(ticket_search, rowid, title, description, labels)
    VALUES ('delete', old.id, old.title, old.description, old.labels);
END;

CREATE TRIGGER tickets_au AFTER UPDATE ON tickets BEGIN
    INSERT INTO ticket_search(ticket_search, rowid, title, description, labels)
    VALUES ('delete', old.id, old.title, old.description, old.labels);
    INSERT INTO ticket_search(rowid, title, description, labels)
    VALUES (new.id, new.title, new.description, new.labels);
END;
