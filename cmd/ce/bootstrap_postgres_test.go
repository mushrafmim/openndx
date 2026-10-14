package main

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// TestBootstrapConsentDatabase exercises the actual psql initialization script
// against a disposable PostgreSQL instance, including repeat runs and quoted names.
func TestBootstrapConsentDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL instance to test bootstrap")
	}
	psql, err := exec.LookPath("psql")
	require.NoError(t, err, "PostgreSQL bootstrap tests require psql")
	adminCfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	admin := stdlib.OpenDB(*adminCfg)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	script, err := os.ReadFile("../../init-db.sql")
	require.NoError(t, err)
	schemaPath, err := filepath.Abs("schema.sql")
	require.NoError(t, err)
	// Substitute the host path for the schema's container mount location.
	bootstrap := strings.ReplaceAll(string(script), "/opt/openndx/ce-schema.sql", schemaPath)

	for _, tc := range []struct {
		name, value, want string
		provided          bool
	}{
		{name: "custom", provided: true, value: "ce_bootstrap_" + uuid.NewString()},
		{name: "quoted", provided: true, value: "ce_bootstrap_" + uuid.NewString() + " 'quoted\""},
		{name: "unset", want: "consent_engine"},
		{name: "empty", provided: true, want: "consent_engine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			want := tc.want
			if want == "" {
				want = tc.value
			}
			var existed bool
			require.NoError(t, admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", want).Scan(&existed))
			if !existed {
				t.Cleanup(func() {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cleanupCancel()
					_, err := admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{want}.Sanitize())
					require.NoError(t, err)
				})
			}
			var defaultExisted bool
			require.NoError(t, admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = 'consent_engine')").Scan(&defaultExisted))
			var env []string
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "DB_NAME_CE=") {
					env = append(env, entry)
				}
			}
			if tc.provided {
				env = append(env, "DB_NAME_CE="+tc.value)
			}
			for range 2 {
				command := exec.CommandContext(ctx, psql, "--no-psqlrc", "--set", "ON_ERROR_STOP=1", "--dbname", dsn)
				command.Env = env
				command.Stdin = strings.NewReader(bootstrap)
				output, err := command.CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			targetCfg := adminCfg.Copy()
			targetCfg.Database = want
			target := stdlib.OpenDB(*targetCfg)
			t.Cleanup(func() { require.NoError(t, target.Close()) })
			var databaseName string
			var table, index sql.NullString
			require.NoError(t, target.QueryRowContext(ctx, `SELECT current_database(),
				to_regclass('public.consent_records')::text, to_regclass('public.idx_consent_active_unique')::text`).
				Scan(&databaseName, &table, &index))
			require.Equal(t, want, databaseName)
			require.True(t, table.Valid, "configured database must contain the consent table")
			require.True(t, index.Valid, "configured database must contain the active-consent index")
			if want != "consent_engine" {
				var defaultExists bool
				require.NoError(t, admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = 'consent_engine')").Scan(&defaultExists))
				require.Equal(t, defaultExisted, defaultExists, "custom bootstrap must not create the default CE database")
			}
		})
	}
}
