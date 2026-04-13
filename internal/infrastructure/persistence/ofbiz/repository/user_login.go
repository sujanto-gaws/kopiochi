package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/domain/ofbizuser"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/ofbiz/models"
)

// ofbizUserLoginRepository implements the ofbizuser.Repository interface
type ofbizUserLoginRepository struct {
	db bun.IDB
}

// NewUserLoginRepository creates a new OFBiz UserLogin repository
func NewUserLoginRepository(db bun.IDB) ofbizuser.Repository {
	return &ofbizUserLoginRepository{db: db}
}

// Create persists a new OFBiz UserLogin
func (r *ofbizUserLoginRepository) Create(ctx context.Context, ul *ofbizuser.UserLogin) error {
	dbModel := toOFBizDBModel(ul)
	_, err := r.db.NewInsert().Model(dbModel).Exec(ctx)
	return err
}

// GetByID retrieves a UserLogin by ID
func (r *ofbizUserLoginRepository) GetByID(ctx context.Context, userLoginID string) (*ofbizuser.UserLogin, error) {
	var dbModel models.OFBizUserLoginDBModel
	err := r.db.NewSelect().
		Model(&dbModel).
		Where("user_login_id = ?", userLoginID).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ofbizuser.ErrUserLoginNotFound
		}
		return nil, err
	}
	return toOFBizDomainEntity(&dbModel), nil
}

// GetByPartyID retrieves all UserLogins associated with a Party
func (r *ofbizUserLoginRepository) GetByPartyID(ctx context.Context, partyID string) ([]*ofbizuser.UserLogin, error) {
	var dbModels []models.OFBizUserLoginDBModel
	err := r.db.NewSelect().
		Model(&dbModels).
		Where("party_id = ?", partyID).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []*ofbizuser.UserLogin{}, nil
		}
		return nil, err
	}

	result := make([]*ofbizuser.UserLogin, len(dbModels))
	for i, dbModel := range dbModels {
		result[i] = toOFBizDomainEntity(&dbModel)
	}
	return result, nil
}

// Update updates an existing UserLogin
func (r *ofbizUserLoginRepository) Update(ctx context.Context, ul *ofbizuser.UserLogin) error {
	dbModel := toOFBizDBModel(ul)
	_, err := r.db.NewUpdate().Model(dbModel).WherePK().Exec(ctx)
	return err
}

// Delete removes a UserLogin by ID
func (r *ofbizUserLoginRepository) Delete(ctx context.Context, userLoginID string) error {
	_, err := r.db.NewDelete().
		Model((*models.OFBizUserLoginDBModel)(nil)).
		Where("user_login_id = ?", userLoginID).
		Exec(ctx)
	return err
}

// UpdatePassword updates the password for a UserLogin
func (r *ofbizUserLoginRepository) UpdatePassword(ctx context.Context, userLoginID string, hashedPassword string) error {
	_, err := r.db.NewUpdate().
		Model((*models.OFBizUserLoginDBModel)(nil)).
		Set("current_password = ?", hashedPassword).
		Set("last_updated_stamp = ?", time.Now()).
		Where("user_login_id = ?", userLoginID).
		Exec(ctx)
	return err
}

// toOFBizDomainEntity converts OFBiz database model to domain entity
func toOFBizDomainEntity(dbModel *models.OFBizUserLoginDBModel) *ofbizuser.UserLogin {
	if dbModel == nil {
		return nil
	}

	isEnabled := dbModel.IsEnabled == "Y"
	requirePasswordChange := dbModel.RequirePasswordChange == "Y"

	return &ofbizuser.UserLogin{
		UserLoginID:            dbModel.UserLoginID,
		CurrentPassword:        dbModel.CurrentPassword,
		PasswordHint:           dbModel.PasswordHint,
		IsEnabled:              isEnabled,
		DisabledDateTime:       dbModel.DisabledDateTime,
		SuccessiveFailedLogins: dbModel.SuccessiveFailedLogins,
		RequirePasswordChange:  requirePasswordChange,
		ExternalAuthID:         dbModel.ExternalAuthID,
		PartyID:                dbModel.PartyID,
		CreatedAt:              dbModel.CreatedStamp,
		UpdatedAt:              dbModel.LastUpdatedStamp,
	}
}

// toOFBizDBModel converts OFBiz domain entity to database model
func toOFBizDBModel(ul *ofbizuser.UserLogin) *models.OFBizUserLoginDBModel {
	if ul == nil {
		return nil
	}

	enabled := "N"
	if ul.IsEnabled {
		enabled = "Y"
	}

	requirePasswordChange := "N"
	if ul.RequirePasswordChange {
		requirePasswordChange = "Y"
	}

	now := time.Now()

	return &models.OFBizUserLoginDBModel{
		UserLoginID:            ul.UserLoginID,
		CurrentPassword:        ul.CurrentPassword,
		PasswordHint:           ul.PasswordHint,
		IsEnabled:              enabled,
		DisabledDateTime:       ul.DisabledDateTime,
		SuccessiveFailedLogins: ul.SuccessiveFailedLogins,
		RequirePasswordChange:  requirePasswordChange,
		ExternalAuthID:         ul.ExternalAuthID,
		PartyID:                ul.PartyID,
		CreatedStamp:           ul.CreatedAt,
		LastUpdatedStamp:       now,
	}
}
