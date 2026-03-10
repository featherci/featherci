---
model: sonnet
---

# Step 03: Database Setup and Migrations

## Objective
Set up SQLite database with schema migrations, using pure Go driver (no CGO).

## Tasks

### 3.1 Add SQLite Dependency
```bash
go get modernc.org/sqlite
go get github.com/jmoiron/sqlx  # For easier queries
```

### 3.2 Create Database Schema
```sql
-- Users table
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,           -- 'github', 'gitlab', 'gitea'
    provider_id TEXT NOT NULL,        -- ID from OAuth provider
    username TEXT NOT NULL,
    email TEXT,
    avatar_url TEXT,
    access_token TEXT,                -- Encrypted
    refresh_token TEXT,               -- Encrypted
    is_admin BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, provider_id)
);

-- Projects table
CREATE TABLE projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,           -- 'github', 'gitlab', 'gitea'
    namespace TEXT NOT NULL,          -- org/user name
    name TEXT NOT NULL,               -- repo name
    full_name TEXT NOT NULL,          -- namespace/name
    clone_url TEXT NOT NULL,
    webhook_secret TEXT,              -- For validating webhooks
    default_branch TEXT DEFAULT 'main',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, full_name)
);

-- Project users (who can access what)
CREATE TABLE project_users (
    project_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    can_manage BOOLEAN DEFAULT FALSE, -- Can edit secrets, settings
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
    encrypted_value BLOB NOT NULL,    -- AES-256-GCM encrypted
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
    build_number INTEGER NOT NULL,    -- Sequential per project
    commit_sha TEXT NOT NULL,
    commit_message TEXT,
    commit_author TEXT,
    branch TEXT,
    pull_request_number INTEGER,
    status TEXT DEFAULT 'pending',    -- pending, running, success, failure, cancelled
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
    image TEXT,                       -- Docker image
    status TEXT DEFAULT 'pending',    -- pending, running, success, failure, skipped, waiting_approval
    exit_code INTEGER,
    started_at DATETIME,
    finished_at DATETIME,
    worker_id TEXT,                   -- Which worker ran this
    log_path TEXT,                    -- Path to log file
    requires_approval BOOLEAN DEFAULT FALSE,
    approved_by INTEGER,              -- User who approved
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
    id TEXT PRIMARY KEY,              -- UUID
    name TEXT NOT NULL,
    status TEXT DEFAULT 'offline',    -- online, offline, busy
    last_heartbeat DATETIME,
    current_step_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (current_step_id) REFERENCES build_steps(id)
);

-- Migrations tracking
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 3.3 Create Migration System
```go
type Migration struct {
    Version int
    Up      string
    Down    string
}

func Migrate(db *sqlx.DB) error
func Rollback(db *sqlx.DB, steps int) error
```

### 3.4 Create Database Package
```go
func Open(path string) (*sqlx.DB, error)
func Close(db *sqlx.DB) error
func Ping(db *sqlx.DB) error
```

### 3.5 Add Indexes
```sql
CREATE INDEX idx_builds_project_id ON builds(project_id);
CREATE INDEX idx_builds_status ON builds(status);
CREATE INDEX idx_build_steps_build_id ON build_steps(build_id);
CREATE INDEX idx_build_steps_status ON build_steps(status);
CREATE INDEX idx_project_users_user_id ON project_users(user_id);
```

### 3.6 Add Tests
- Test database creation
- Test migrations apply correctly
- Test indexes exist
- Test foreign keys work

## Deliverables
- [ ] `internal/database/database.go` - Connection management
- [ ] `internal/database/migrations.go` - Migration system
- [ ] `internal/database/schema.go` - Schema definitions
- [ ] `internal/database/database_test.go` - Tests
- [ ] All tables created with proper constraints

## Dependencies
- Step 02: Configuration (for database path)

## Estimated Effort
Medium - Core data structures
