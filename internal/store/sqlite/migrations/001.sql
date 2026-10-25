CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type = 'git'),
    state TEXT NOT NULL CHECK(state IN ('creating', 'ready', 'failed', 'deleting')),
    version INTEGER NOT NULL CHECK(version > 0),
    error TEXT NOT NULL DEFAULT '',
    creator_token TEXT NOT NULL DEFAULT ''
);
PRAGMA user_version = 1;
