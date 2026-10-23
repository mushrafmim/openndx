package main

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// autoMigrate runs GORM auto-migration for the given models one at a time, in
// the order given. Callers must list referenced models before the models that
// reference them, so tables exist before GORM creates foreign keys to them.
func autoMigrate(db *gorm.DB, models ...any) error {
	slog.Info("Running GORM auto-migration")
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("failed to run auto-migration for %T: %w", m, err)
		}
	}
	slog.Info("GORM auto-migration completed successfully")
	return nil
}
