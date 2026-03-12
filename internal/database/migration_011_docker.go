package database

func init() {
	RegisterMigration(Migration{
		Version:     11,
		Description: "Add docker column to build_steps",
		Up: `
ALTER TABLE build_steps ADD COLUMN docker BOOLEAN NOT NULL DEFAULT 0;
`,
		Down: `
ALTER TABLE build_steps DROP COLUMN docker;
`,
	})
}
