// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"testing"

	"database/sql"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func TestMigrateBlueskyApplicationFilter(t *testing.T) {
	ctx := context.Background()
	raw, err := sql.Open("sqlite-bluesky-app-password-test", ":memory:")
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	database := bun.NewDB(raw, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.ExecContext(ctx, "CREATE TABLE bluesky_connections (id TEXT PRIMARY KEY, crosspost_public BOOLEAN NOT NULL)")
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, "INSERT INTO bluesky_connections VALUES ('existing', TRUE)")
	require.NoError(t, err)
	require.NoError(t, migrateBlueskyApplicationFilter(ctx, database))
	var enabled bool
	require.NoError(t, database.NewSelect().Table("bluesky_connections").Column("crosspost_application_filter").Scan(ctx, &enabled))
	require.False(t, enabled, "upgrade preserves all-app crossposting")
	var ids *string
	require.NoError(t, database.NewSelect().Table("bluesky_connections").Column("crosspost_application_ids").Scan(ctx, &ids))
	require.Nil(t, ids)
}
