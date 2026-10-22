// Package db creates the PDP's PostgreSQL schema.
package db

import (
	"fmt"
	"log/slog"

	"github.com/openndx/openndx-core/internal/pdp/models"
	"gorm.io/gorm"
)

// enumTypes are created explicitly because GORM does not create PostgreSQL
// enum types on its own. Each statement is a no-op when the type exists.
var enumTypes = []string{
	`DO $$ BEGIN
		CREATE TYPE source_enum AS ENUM ('primary', 'fallback');
	EXCEPTION
		WHEN duplicate_object THEN null;
	END $$;`,
	`DO $$ BEGIN
		CREATE TYPE access_control_type_enum AS ENUM ('public', 'restricted');
	EXCEPTION
		WHEN duplicate_object THEN null;
	END $$;`,
	`DO $$ BEGIN
		CREATE TYPE owner_enum AS ENUM ('citizen');
	EXCEPTION
		WHEN duplicate_object THEN null;
	END $$;`,
}

// Migrate creates the enum types and the policy_metadata table on a
// PostgreSQL database. It is safe to run on every start.
func Migrate(db *gorm.DB) error {
	slog.Info("Running GORM auto-migration for PDP models")
	for _, enumQuery := range enumTypes {
		if err := db.Exec(enumQuery).Error; err != nil {
			return fmt.Errorf("failed to create enum type: %w", err)
		}
	}
	if err := db.AutoMigrate(&models.PolicyMetadata{}); err != nil {
		return fmt.Errorf("failed to run auto-migration: %w", err)
	}
	slog.Info("GORM auto-migration completed successfully")
	return nil
}
