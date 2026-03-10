package database

func init() {
	RegisterMigration(Migration{
		Version:     4,
		Description: "Add step metadata columns for workflow execution",
		Up: `
-- Add columns for storing step execution metadata
ALTER TABLE build_steps ADD COLUMN commands_json TEXT DEFAULT '[]';
ALTER TABLE build_steps ADD COLUMN env_json TEXT DEFAULT '{}';
ALTER TABLE build_steps ADD COLUMN depends_on_json TEXT DEFAULT '[]';
ALTER TABLE build_steps ADD COLUMN working_dir TEXT DEFAULT '';
ALTER TABLE build_steps ADD COLUMN timeout_minutes INTEGER DEFAULT 60;
`,
		Down: `
-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
-- For simplicity in dev, we'll just leave these columns in rollback
-- In production, you'd want to recreate the table without these columns
`,
	})
}
