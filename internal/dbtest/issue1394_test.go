package dbtest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

type issue1394Model struct {
	bun.BaseModel `bun:"table:issue_1394_models,alias:im"`

	ID    int64  `bun:",pk,autoincrement"`
	Name  string `bun:",notnull"`
	Email string `bun:",notnull,default:'none'"`
}

type issue1394NoDefaultModel struct {
	bun.BaseModel `bun:"table:issue_1394_no_default_models,alias:im"`

	ID     int64  `bun:",pk,autoincrement"`
	Name   string `bun:",notnull"`
	Status string `bun:",notnull,nullzero"`
}

// TestIssue1394SliceInsertQuery checks the generated SQL: no dialect may drop
// a value that a later row of a slice insert carries for a field that the
// first row leaves to the column default.
func TestIssue1394SliceInsertQuery(t *testing.T) {
	testEachDB(t, func(t *testing.T, dbName string, db *bun.DB) {
		t.Run("a later row's value is kept", func(t *testing.T) {
			models := []issue1394Model{
				{Name: "zero-email"},
				{Name: "with-email", Email: "alice@example.com"},
			}

			query, err := db.NewInsert().Model(&models).AppendQuery(db.QueryGen(), nil)
			require.NoError(t, err)
			require.Contains(t, string(query), "'alice@example.com'")

			if db.Dialect().Name() == dialect.SQLite {
				require.Contains(t, string(query), "'none'",
					"without a DEFAULT placeholder the declared default stands in for the first row")
			}
		})

		t.Run("the column is left out when every row takes the default", func(t *testing.T) {
			if db.Dialect().Name() != dialect.SQLite {
				t.Skip("only dialects without a DEFAULT placeholder leave the column out")
			}

			models := []issue1394Model{{Name: "first"}, {Name: "second"}}

			query, err := db.NewInsert().Model(&models).AppendQuery(db.QueryGen(), nil)
			require.NoError(t, err)
			require.NotContains(t, string(query), "'none'")
			require.Contains(t, string(query), `RETURNING "id", "email"`)
		})

		t.Run("a mixed field without a declared default", func(t *testing.T) {
			models := []issue1394NoDefaultModel{
				{Name: "first"},
				{Name: "second", Status: "active"},
			}

			query, err := db.NewInsert().Model(&models).AppendQuery(db.QueryGen(), nil)
			if db.Dialect().Name() == dialect.SQLite {
				require.ErrorContains(t, err, "field Status takes the column default in some rows")
				return
			}
			require.NoError(t, err)
			require.Contains(t, string(query), "'active'")
		})
	})
}

// TestIssue1394SliceInsert inserts through SQLite, the dialect without a DEFAULT
// placeholder, and reads the rows back.
func TestIssue1394SliceInsert(t *testing.T) {
	ctx := context.Background()
	db := sqlite(t)

	insert := func(t *testing.T, models []issue1394Model) []issue1394Model {
		t.Helper()

		mustResetModel(t, ctx, db, (*issue1394Model)(nil))

		_, err := db.NewInsert().Model(&models).Exec(ctx)
		require.NoError(t, err)

		var got []issue1394Model
		err = db.NewSelect().Model(&got).OrderExpr("im.id ASC").Scan(ctx)
		require.NoError(t, err)
		return got
	}

	t.Run("the first row takes the default and a later row has a value", func(t *testing.T) {
		got := insert(t, []issue1394Model{
			{Name: "zero-email"},
			{Name: "with-email", Email: "alice@example.com"},
		})

		require.Len(t, got, 2)
		require.Equal(t, "none", got[0].Email)
		require.Equal(t, "alice@example.com", got[1].Email)
	})

	t.Run("the first row has a value and a later row takes the default", func(t *testing.T) {
		got := insert(t, []issue1394Model{
			{Name: "with-email", Email: "alice@example.com"},
			{Name: "zero-email"},
		})

		require.Len(t, got, 2)
		require.Equal(t, "alice@example.com", got[0].Email)
		require.Equal(t, "none", got[1].Email)
	})

	t.Run("every row takes the default", func(t *testing.T) {
		mustResetModel(t, ctx, db, (*issue1394Model)(nil))

		models := []issue1394Model{{Name: "first"}, {Name: "second"}}
		_, err := db.NewInsert().Model(&models).Exec(ctx)
		require.NoError(t, err)

		require.Equal(t, "none", models[0].Email, "RETURNING scans the default back")
		require.Equal(t, "none", models[1].Email)
	})
}
