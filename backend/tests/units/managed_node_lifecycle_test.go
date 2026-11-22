package units

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/platform/database"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestManagedNodeLifecycleArchivesAccountsAndPreservesIndependentLocks(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	originalDB := database.PostgresDB
	database.PostgresDB = db
	t.Cleanup(func() { database.PostgresDB = originalDB })

	ctx := context.Background()
	repo := repository.NewPortalRepository()
	local := models.PortalLocalAccount{CustomerID: "018f5485-0000-7000-8000-000000000001", OcservID: 7}
	accountUpdate := `UPDATE "ocserv_users" SET "is_locked"=\$1,"updated_at"=\$2 WHERE \(id = \$3 AND is_locked <> \$4\) AND "ocserv_users"\."deleted_at" IS NULL`
	bindingUpdate := `UPDATE "portal_local_accounts" SET "lock_applied"=\$1 WHERE "customer_id" = \$2`

	mock.ExpectBegin()
	mock.ExpectExec(accountUpdate).WithArgs(true, sqlmock.AnyArg(), local.OcservID, true).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(bindingUpdate).WithArgs(true, local.CustomerID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.ApplyNodeLock(ctx, local, true))

	local.LockApplied = true
	mock.ExpectBegin()
	mock.ExpectExec(accountUpdate).WithArgs(false, sqlmock.AnyArg(), local.OcservID, false).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(bindingUpdate).WithArgs(false, local.CustomerID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.ApplyNodeLock(ctx, local, false))

	local.LockApplied = false
	mock.ExpectBegin()
	mock.ExpectExec(accountUpdate).WithArgs(true, sqlmock.AnyArg(), local.OcservID, true).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	require.NoError(t, repo.ApplyNodeLock(ctx, local, true))
	require.NoError(t, repo.ApplyNodeLock(ctx, local, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ocserv_users" SET "deleted_at"=\$1 WHERE "ocserv_users"\."id" = \$2 AND "ocserv_users"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), local.OcservID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.ArchiveNodeAccount(ctx, local.OcservID))
	require.NoError(t, mock.ExpectationsWereMet())
}
