CREATE TABLE projects_next (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type = 'git'),
    state TEXT NOT NULL CHECK(state IN (
        'creating', 'ready', 'failed', 'deleting', 'delete_failed'
    )),
    version INTEGER NOT NULL CHECK(version > 0),
    error TEXT NOT NULL DEFAULT '',
    creator_token TEXT NOT NULL DEFAULT ''
);

INSERT INTO projects_next (id, name, type, state, version, error, creator_token)
SELECT id, name, type, state, version, error, creator_token FROM projects;

DROP TABLE projects;
ALTER TABLE projects_next RENAME TO projects;
PRAGMA user_version = 2;
