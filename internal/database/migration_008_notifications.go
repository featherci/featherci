package database

func init() {
	RegisterMigration(Migration{
		Version:     8,
		Description: "Add notification_channels table",
		Up: `
CREATE TABLE notification_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config_encrypted BLOB NOT NULL,
    on_success BOOLEAN NOT NULL DEFAULT 0,
    on_failure BOOLEAN NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_by INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
);
CREATE INDEX idx_notification_channels_project_id ON notification_channels(project_id);
`,
		Down: `
DROP TABLE IF EXISTS notification_channels;
`,
	})
}
