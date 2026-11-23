package units

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/platform/database"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestManagedNodeLifecyclePurgesAccountsAndPreservesIndependentLocks(t *testing.T) {
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

	account := &models.OcservUser{ID: local.OcservID, Username: "managed", CreatedAt: time.Now().UTC()}
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "ocserv_users" WHERE "ocserv_users"\."id" = \$1`).
		WithArgs(local.OcservID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "ocserv_user_session_logs" WHERE portal_account_id = \$1 OR \(portal_account_id IS NULL AND username = \$2 AND created_at >= \$3\)`).
		WithArgs(local.OcservID, account.Username, account.CreatedAt).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "portal_local_accounts" WHERE "portal_local_accounts"\."customer_id" = \$1`).
		WithArgs(local.CustomerID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.PurgeNodeAccount(ctx, local, account))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompletedCustomerPurgeDeletesMasterRecordsTogether(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	originalDB := database.PostgresDB
	database.PostgresDB = db
	t.Cleanup(func() { database.PostgresDB = originalDB })

	ctx := context.Background()
	store := repository.NewPortalRepository()
	id := "018f5485-0000-7000-8000-000000000001"
	mock.ExpectQuery(`(?s)SELECT .*FROM "portal_customers".*NOT EXISTS.*a\.customer_id = portal_customers\.id.*NOT a\.remove OR a\.version <> a\.applied`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	customers, err := store.CompletedCustomerDeletions(ctx)
	require.NoError(t, err)
	require.Len(t, customers, 1)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(708154631\)`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "portal_client_certificates" WHERE customer_id = \$1`).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "portal_assignments" WHERE customer_id = \$1`).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM "portal_customers" WHERE id = \$1 AND deleted_at IS NOT NULL`).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		return tx.PurgeCustomer(ctx, customers[0].ID)
	}))
	require.NoError(t, mock.ExpectationsWereMet())
}
