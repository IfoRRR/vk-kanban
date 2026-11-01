-- Модель мониторинга. Рабочий процесс хранит те же сущности в UPTIME_FILE.

CREATE TABLE services (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    url     TEXT NOT NULL,
    kind    TEXT NOT NULL CHECK (kind IN ('http', 'body')),
    expect  TEXT NOT NULL DEFAULT '',
    seed    INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE checks (
    id         INTEGER PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id),
    at         TEXT NOT NULL,
    ok         INTEGER NOT NULL,
    ms         INTEGER NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    code       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE incidents (
    id         TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id),
    opened     TEXT NOT NULL,
    closed     TEXT,
    error      TEXT NOT NULL
);
