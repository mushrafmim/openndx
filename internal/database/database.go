// Package database provides shared PostgreSQL adapters for database/sql pools.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Wrap creates a GORM handle over an already opened PostgreSQL pool.
// The caller owns the pool and must close it on startup failure or shutdown.
func Wrap(sqlDB *sql.DB) (*gorm.DB, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("SQL database is required")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
			SlowThreshold: 200 * time.Millisecond, LogLevel: logger.Warn,
			IgnoreRecordNotFoundError: true, ParameterizedQueries: true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize GORM adapter: %w", err)
	}
	return db, nil
}
