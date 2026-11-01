-- Модель бронирования переговорных. Рабочий процесс хранит те же
-- сущности в ROOMS_FILE.

CREATE TABLE rooms (
    id    TEXT PRIMARY KEY,
    name  TEXT NOT NULL,
    seats INTEGER NOT NULL,
    floor TEXT NOT NULL
);

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    role          TEXT NOT NULL CHECK (role IN ('employee', 'admin')),
    password_hash TEXT NOT NULL,
    salt          TEXT NOT NULL,
    token         TEXT,
    created_at    INTEGER NOT NULL
);

CREATE TABLE bookings (
    id       TEXT PRIMARY KEY,
    room_id  TEXT NOT NULL REFERENCES rooms(id),
    owner_id TEXT REFERENCES users(id),
    date     TEXT NOT NULL,
    start    TEXT NOT NULL,
    minutes  INTEGER NOT NULL CHECK (minutes IN (30, 60, 90)),
    who      TEXT NOT NULL,
    title    TEXT NOT NULL
);
