package database

func init() {
	RegisterMigration(Migration{
		Version:     5,
		Description: "Add cache_json column to build_steps",
		Up: `
ALTER TABLE build_steps ADD COLUMN cache_json TEXT DEFAULT '';
`,
		Down: `
-- SQLite doesn't support DROP COLUMN directly
`,
	})
}
