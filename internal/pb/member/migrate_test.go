package member

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/stretchr/testify/assert"
)

func TestDropLegacyColumns(t *testing.T) {
	t.Run("NoTable", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		mock.ExpectQuery(`information_schema.tables`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		assert.NoError(t, DropLegacyColumns(db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DropsIdpUserID", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		mock.ExpectQuery(`information_schema.tables`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`(?i)information_schema.columns`).
			WithArgs("members", "idp_user_id").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectExec(`ALTER TABLE "members" DROP COLUMN "idp_user_id"`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		assert.NoError(t, DropLegacyColumns(db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("AlreadyDropped", func(t *testing.T) {
		db, mock, cleanup := dbtest.SetupMockDB(t)
		defer cleanup()

		mock.ExpectQuery(`information_schema.tables`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`(?i)information_schema.columns`).
			WithArgs("members", "idp_user_id").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		assert.NoError(t, DropLegacyColumns(db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
