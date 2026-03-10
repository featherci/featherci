package database

func init() {
	RegisterMigration(Migration{
		Version:     2,
		Description: "Add indexes for common queries",
		Up: `
CREATE INDEX idx_builds_project_id ON builds(project_id);
CREATE INDEX idx_builds_status ON builds(status);
CREATE INDEX idx_builds_created_at ON builds(created_at);
CREATE INDEX idx_build_steps_build_id ON build_steps(build_id);
CREATE INDEX idx_build_steps_status ON build_steps(status);
CREATE INDEX idx_project_users_user_id ON project_users(user_id);
CREATE INDEX idx_secrets_project_id ON secrets(project_id);
CREATE INDEX idx_users_username ON users(username);
`,
		Down: `
DROP INDEX IF EXISTS idx_builds_project_id;
DROP INDEX IF EXISTS idx_builds_status;
DROP INDEX IF EXISTS idx_builds_created_at;
DROP INDEX IF EXISTS idx_build_steps_build_id;
DROP INDEX IF EXISTS idx_build_steps_status;
DROP INDEX IF EXISTS idx_project_users_user_id;
DROP INDEX IF EXISTS idx_secrets_project_id;
DROP INDEX IF EXISTS idx_users_username;
`,
	})
}
