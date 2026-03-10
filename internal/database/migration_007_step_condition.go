package database

func init() {
	RegisterMigration(Migration{
		Version:     7,
		Description: "Add condition_expr column to build_steps",
		Up: `
ALTER TABLE build_steps ADD COLUMN condition_expr TEXT DEFAULT '';
`,
		Down: `
-- SQLite doesn't support DROP COLUMN directly
`,
	})
}
