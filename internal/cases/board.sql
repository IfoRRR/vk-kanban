-- Модель канбана. Рабочий процесс хранит те же сущности в BOARD_FILE.

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    salt          TEXT NOT NULL,
    token         TEXT,
    created_at    INTEGER NOT NULL
);

CREATE TABLE boards (
    id       TEXT PRIMARY KEY,
    title    TEXT NOT NULL,
    owner_id TEXT REFERENCES users(id),
    public   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE board_members (
    board_id TEXT NOT NULL REFERENCES boards(id),
    user_id  TEXT NOT NULL REFERENCES users(id),
    PRIMARY KEY (board_id, user_id)
);

CREATE TABLE columns (
    id       TEXT PRIMARY KEY,
    board_id TEXT NOT NULL REFERENCES boards(id),
    title    TEXT NOT NULL,
    position INTEGER NOT NULL
);

CREATE TABLE cards (
    id          TEXT PRIMARY KEY,
    board_id    TEXT NOT NULL REFERENCES boards(id),
    column_id   TEXT NOT NULL REFERENCES columns(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    assignee    TEXT NOT NULL DEFAULT '',
    due         TEXT NOT NULL DEFAULT '',
    position    INTEGER NOT NULL
);

CREATE TABLE card_tags (
    card_id TEXT NOT NULL REFERENCES cards(id),
    tag     TEXT NOT NULL,
    PRIMARY KEY (card_id, tag)
);

CREATE TABLE comments (
    id      TEXT PRIMARY KEY,
    card_id TEXT NOT NULL REFERENCES cards(id),
    author  TEXT NOT NULL,
    body    TEXT NOT NULL,
    at      TEXT NOT NULL
);

CREATE TABLE checks (
    id      TEXT PRIMARY KEY,
    card_id TEXT NOT NULL REFERENCES cards(id),
    text    TEXT NOT NULL,
    done    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE links (
    card_id TEXT NOT NULL REFERENCES cards(id),
    name    TEXT NOT NULL,
    url     TEXT NOT NULL
);

CREATE TABLE events (
    board_id TEXT NOT NULL REFERENCES boards(id),
    at       TEXT NOT NULL,
    text     TEXT NOT NULL
);
