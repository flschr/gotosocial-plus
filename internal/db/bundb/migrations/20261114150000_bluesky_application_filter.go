// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/uptrace/bun"
)

func init() {
	if err := Migrations.Register(migrateBlueskyApplicationFilter, func(context.Context, *bun.DB) error { return nil }); err != nil {
		panic(err)
	}
}

func migrateBlueskyApplicationFilter(ctx context.Context, db *bun.DB) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewAddColumn().Model((*gtsmodel.BlueskyConnection)(nil)).ColumnExpr("crosspost_application_filter BOOLEAN NOT NULL DEFAULT FALSE").Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewAddColumn().Model((*gtsmodel.BlueskyConnection)(nil)).ColumnExpr("crosspost_application_ids JSONB").Exec(ctx)
		return err
	})
}
