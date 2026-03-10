package database

func init() {
	RegisterMigration(Migration{
		Version:     9,
		Description: "Add on_cancelled column to notification_channels",
		Up: `
ALTER TABLE notification_channels ADD COLUMN on_cancelled BOOLEAN NOT NULL DEFAULT 0;
`,
		Down: `
ALTER TABLE notification_channels DROP COLUMN on_cancelled;
`,
	})
}
