package database

func init() {
	RegisterMigration(Migration{
		Version:     6,
		Description: "Add webhook_id column to projects",
		Up: `
ALTER TABLE projects ADD COLUMN webhook_id TEXT DEFAULT '';
`,
		Down: `
-- SQLite doesn't support DROP COLUMN directly
`,
	})
}
