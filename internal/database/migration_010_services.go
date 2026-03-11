package database

func init() {
	RegisterMigration(Migration{
		Version:     10,
		Description: "Add services_json column to build_steps",
		Up: `
ALTER TABLE build_steps ADD COLUMN services_json TEXT NOT NULL DEFAULT '';
`,
		Down: `
ALTER TABLE build_steps DROP COLUMN services_json;
`,
	})
}
