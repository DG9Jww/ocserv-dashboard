package repository

import (
	"context"
	"errors"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/platform/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PortalRepository struct{ db *gorm.DB }

func NewPortalRepository() *PortalRepository { return &PortalRepository{db: database.GetConnection()} }

func (r *PortalRepository) Transaction(ctx context.Context, fn func(*PortalRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(708154631)").Error; err != nil {
			return err
		}
		return fn(&PortalRepository{db: tx})
	})
}

func (r *PortalRepository) Save(ctx context.Context, value any) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(value).Error
}

func (r *PortalRepository) Customers(ctx context.Context) ([]models.PortalCustomer, error) {
	var customers []models.PortalCustomer
	err := r.db.WithContext(ctx).Order("username").Find(&customers).Error
	return customers, err
}

func (r *PortalRepository) Customer(ctx context.Context, id string) (models.PortalCustomer, error) {
	var customer models.PortalCustomer
	err := r.db.WithContext(ctx).First(&customer, "id = ?", id).Error
	return customer, err
}

func (r *PortalRepository) ByUsername(ctx context.Context, name string) (models.PortalCustomer, error) {
	var customer models.PortalCustomer
	err := r.db.WithContext(ctx).First(&customer, "username = ?", name).Error
	return customer, err
}

func (r *PortalRepository) Groups(ctx context.Context) ([]models.PortalNodeGroup, error) {
	var groups []models.PortalNodeGroup
	err := r.db.WithContext(ctx).Order("name").Find(&groups).Error
	return groups, err
}

func (r *PortalRepository) Defaults(ctx context.Context) ([]models.PortalNodeDefault, error) {
	var defaults []models.PortalNodeDefault
	err := r.db.WithContext(ctx).Find(&defaults).Error
	return defaults, err
}

func (r *PortalRepository) Assignments(ctx context.Context, id string) ([]models.PortalAssignment, error) {
	var assignments []models.PortalAssignment
	query := r.db.WithContext(ctx).Order("node, customer_id")
	if id != "" {
		query = query.Where("customer_id = ?", id)
	}
	err := query.Find(&assignments).Error
	return assignments, err
}

func (r *PortalRepository) Jobs(ctx context.Context, now time.Time) ([]models.PortalAssignment, error) {
	var jobs []models.PortalAssignment
	err := r.db.WithContext(ctx).Where("version > applied AND retry_at <= ?", now).
		Order("desired ASC, retry_at ASC, customer_id, node").Limit(32).Find(&jobs).Error
	return jobs, err
}

func (r *PortalRepository) Finish(ctx context.Context, job models.PortalAssignment, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.PortalAssignment{}).
		Where("customer_id = ? AND node = ? AND version = ?", job.CustomerID, job.Node, job.Version).
		Updates(fields).Error
}

func (r *PortalRepository) Local(ctx context.Context, customerID string) (models.PortalLocalAccount, error) {
	var local models.PortalLocalAccount
	err := r.db.WithContext(ctx).First(&local, "customer_id = ?", customerID).Error
	return local, err
}

func (r *PortalRepository) Reserve(ctx context.Context, customerID string, account *models.OcservUser, reserved bool) (models.PortalLocalAccount, error) {
	result := models.PortalLocalAccount{CustomerID: customerID}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(account).Error; err != nil {
			return err
		}
		result.OcservID = account.ID
		if reserved {
			return tx.Model(&result).Update("ocserv_id", account.ID).Error
		}
		return tx.Create(&result).Error
	})
	return result, err
}

func (r *PortalRepository) CA(ctx context.Context, scope string) (models.PortalSigningCA, error) {
	var ca models.PortalSigningCA
	err := r.db.WithContext(ctx).First(&ca, "scope = ?", scope).Error
	return ca, err
}

func (r *PortalRepository) Certificate(ctx context.Context, id, scope string) (models.PortalClientCertificate, error) {
	var cert models.PortalClientCertificate
	err := r.db.WithContext(ctx).First(&cert, "customer_id = ? AND scope = ?", id, scope).Error
	return cert, err
}

func (r *PortalRepository) SaveNew(ctx context.Context, value any) error {
	return r.db.WithContext(ctx).Create(value).Error
}

func (r *PortalRepository) NodeActivity(ctx context.Context, accountID uint, from, until *time.Time, offset, limit int) ([]models.OcservUserSessionLog, int64, error) {
	items := make([]models.OcservUserSessionLog, 0)
	query := r.db.WithContext(ctx).Model(&models.OcservUserSessionLog{}).Where("portal_account_id = ?", accountID)
	if from != nil {
		query = query.Where("created_at >= ?", *from)
	}
	if until != nil {
		query = query.Where("created_at <= ?", *until)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit == 0 {
		return items, total, nil
	}
	err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func (r *PortalRepository) MarkLogAccount(ctx context.Context, log *models.OcservUserSessionLog) error {
	var account struct{ ID uint }
	err := r.db.WithContext(ctx).Table("portal_local_accounts").
		Select("portal_local_accounts.ocserv_id AS id").
		Joins("JOIN ocserv_users ON ocserv_users.id = portal_local_accounts.ocserv_id").
		Where("ocserv_users.username = ?", log.Username).Take(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err == nil {
		log.PortalAccountID = &account.ID
	}
	return err
}

func (r *PortalRepository) NodeDaily(ctx context.Context, accountID uint, from, until *time.Time) ([]models.DailyTraffic, error) {
	result := make([]models.DailyTraffic, 0)
	query := r.db.WithContext(ctx).Model(&models.OcservUserTrafficStatistics{}).Where("ocserv_user_id = ?", accountID)
	if from != nil {
		query = query.Where("created_at >= ?", *from)
	}
	if until != nil {
		query = query.Where("created_at <= ?", *until)
	}
	err := query.Select("TO_CHAR(DATE(created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS date, SUM(rx)/1073741824.0 AS rx, SUM(tx)/1073741824.0 AS tx").
		Group("DATE(created_at AT TIME ZONE 'UTC')").Order("date").Scan(&result).Error
	return result, err
}
