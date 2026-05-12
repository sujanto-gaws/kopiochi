package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/auth/models"
	"github.com/uptrace/bun"
)

// codeVerifier abstracts hash comparison so the repository layer stays free of
// crypto primitives. BcryptHasher satisfies this interface automatically.
type codeVerifier interface {
	Verify(plain, hashed string) bool
}

type MFAStore struct {
	db       bun.IDB
	verifier codeVerifier
}

func NewMFAStore(db bun.IDB, verifier codeVerifier) *MFAStore {
	return &MFAStore{db: db, verifier: verifier}
}

func (s *MFAStore) StoreBackupCodes(ctx context.Context, userID string, codeHashes []string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return err
	}
	// Use bun’s bulk insert
	rows := make([]models.MfaBackupCodeRow, len(codeHashes))
	for i, hash := range codeHashes {
		rows[i] = models.MfaBackupCodeRow{
			UserID:   uid,
			CodeHash: hash,
		}
	}
	_, err = s.db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

func (s *MFAStore) FindAndUseBackupCode(ctx context.Context, userID string, plainCode string) (bool, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, err
	}
	// Fetch all unused codes for the user
	var codes []models.MfaBackupCodeRow
	err = s.db.NewSelect().
		Model(&codes).
		Where("user_id = ?", uid).
		Where("used = false").
		Scan(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range codes {
		if s.verifier.Verify(plainCode, c.CodeHash) {
			// Mark as used
			_, err := s.db.NewUpdate().
				Model(&c).
				Set("used = true").
				WherePK().
				Exec(ctx)
			if err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}
