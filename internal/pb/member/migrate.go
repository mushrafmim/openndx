package member

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// DropLegacyColumns removes columns from the pre-organization member schema,
// when members were tied to a single IdP user. AutoMigrate only adds columns,
// so this must run before it.
func DropLegacyColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable(&Member{}) || !db.Migrator().HasColumn(&Member{}, "idp_user_id") {
		return nil
	}
	if err := db.Migrator().DropColumn(&Member{}, "idp_user_id"); err != nil {
		return fmt.Errorf("failed to drop members.idp_user_id: %w", err)
	}
	slog.Info("Dropped legacy column members.idp_user_id")
	return nil
}
