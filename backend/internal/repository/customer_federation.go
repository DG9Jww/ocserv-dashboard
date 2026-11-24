package repository

import (
	"context"
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

func (r *PortalRepository) SaveNew(ctx context.Context, value any) error {
	return r.db.WithContext(ctx).Create(value).Error
}

func (r *PortalRepository) UpdateCustomer(ctx context.Context, customer *models.PortalCustomer) error {
	return r.db.WithContext(ctx).Model(customer).
		Select("Locked", "DeletedAt", "PasswordHash", "PolicyGroupID", "Node", "DefaultNode", "TrafficType", "TrafficSize", "ExpiresAt").
		Updates(customer).Error
}

func (r *PortalRepository) Customers(ctx context.Context) ([]models.PortalCustomer, error) {
	var customers []models.PortalCustomer
	err := r.db.WithContext(ctx).Order("username").Find(&customers).Error
	return customers, err
}

func (r *PortalRepository) CustomersByPolicy(ctx context.Context, id string) ([]models.PortalCustomer, error) {
	var customers []models.PortalCustomer
	err := r.db.WithContext(ctx).Where("policy_group_id = ?", id).Find(&customers).Error
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

func (r *PortalRepository) PolicyGroups(ctx context.Context) ([]models.PortalPolicyGroup, error) {
	var groups []models.PortalPolicyGroup
	err := r.db.WithContext(ctx).Order("name").Find(&groups).Error
	return groups, err
}

func (r *PortalRepository) PolicyGroup(ctx context.Context, id string) (models.PortalPolicyGroup, error) {
	var group models.PortalPolicyGroup
	err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error
	return group, err
}

func (r *PortalRepository) DeletePolicyGroup(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.PortalPolicyGroup{}, "id = ?", id).Error
}

func (r *PortalRepository) PolicyGroupInUse(ctx context.Context, id string) (bool, error) {
	var inUse bool
	err := r.db.WithContext(ctx).Raw("SELECT EXISTS (SELECT 1 FROM portal_customers WHERE policy_group_id = ?)", id).Scan(&inUse).Error
	return inUse, err
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

func (r *PortalRepository) CompletedCustomerDeletions(ctx context.Context) ([]models.PortalCustomer, error) {
	var customers []models.PortalCustomer
	err := r.db.WithContext(ctx).Where(`deleted_at IS NOT NULL AND NOT EXISTS (
		SELECT 1 FROM portal_assignments AS a WHERE a.customer_id = portal_customers.id
		AND (NOT a.remove OR a.version <> a.applied)
	)`).Order("deleted_at").Limit(32).Find(&customers).Error
	return customers, err
}

func (r *PortalRepository) PurgeCustomer(ctx context.Context, id string) error {
	db := r.db.WithContext(ctx)
	if err := db.Delete(&models.PortalAssignment{}, "customer_id = ?", id).Error; err != nil {
		return err
	}
	return db.Delete(&models.PortalCustomer{}, "id = ? AND deleted_at IS NOT NULL", id).Error
}
