package database

func init() {
	RegisterMigration(Migration{
		Version:     1,
		Description: "Create initial schema",
		Up: `
-- Users table
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    username TEXT NOT NULL,
    email TEXT,
    avatar_url TEXT,
    access_token TEXT,
    refresh_token TEXT,
    is_admin BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, provider_id)
);

-- Projects table
CREATE TABLE projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    namespace TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    webhook_secret TEXT,
    default_branch TEXT DEFAULT 'main',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, full_name)
);

-- Project users (who can access what)
CREATE TABLE project_users (
    project_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    can_manage BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, user_id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Secrets table
CREATE TABLE secrets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    encrypted_value BLOB NOT NULL,
    created_by INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, name),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
);

-- Builds table
CREATE TABLE builds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    build_number INTEGER NOT NULL,
    commit_sha TEXT NOT NULL,
    commit_message TEXT,
    commit_author TEXT,
    branch TEXT,
    pull_request_number INTEGER,
    status TEXT DEFAULT 'pending',
    started_at DATETIME,
    finished_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, build_number),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Build steps table
CREATE TABLE build_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    image TEXT,
    status TEXT DEFAULT 'pending',
    exit_code INTEGER,
    started_at DATETIME,
    finished_at DATETIME,
    worker_id TEXT,
    log_path TEXT,
    requires_approval BOOLEAN DEFAULT FALSE,
    approved_by INTEGER,
    approved_at DATETIME,
    FOREIGN KEY (build_id) REFERENCES builds(id) ON DELETE CASCADE,
    FOREIGN KEY (approved_by) REFERENCES users(id)
);

-- Step dependencies
CREATE TABLE step_dependencies (
    step_id INTEGER NOT NULL,
    depends_on_step_id INTEGER NOT NULL,
    PRIMARY KEY (step_id, depends_on_step_id),
    FOREIGN KEY (step_id) REFERENCES build_steps(id) ON DELETE CASCADE,
    FOREIGN KEY (depends_on_step_id) REFERENCES build_steps(id) ON DELETE CASCADE
);

-- Workers table
CREATE TABLE workers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT DEFAULT 'offline',
    last_heartbeat DATETIME,
    current_step_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (current_step_id) REFERENCES build_steps(id)
);
`,
		Down: `
DROP TABLE IF EXISTS workers;
DROP TABLE IF EXISTS step_dependencies;
DROP TABLE IF EXISTS build_steps;
DROP TABLE IF EXISTS builds;
DROP TABLE IF EXISTS secrets;
DROP TABLE IF EXISTS project_users;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;
`,
	})
}
