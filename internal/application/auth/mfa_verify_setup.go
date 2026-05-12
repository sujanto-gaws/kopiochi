package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) VerifyMFASetup(ctx context.Context, userID string, code string) (*MfaVerifySetupResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !s.mfaService.ValidateCode(user.MFASecret, code) {
		return nil, ErrInvalidMFACode
	}
	// Generate backup codes
	backupCodes := make([]string, 8)
	codeHashes := make([]string, 8)
	for i := 0; i < 8; i++ {
		b := make([]byte, 4)
		rand.Read(b)
		code := hex.EncodeToString(b)[:8]  // 8-digit hex code
		backupCodes[i] = code
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash backup: %w", err)
		}
		codeHashes[i] = string(hash)
	}
	if err := s.mfaStore.StoreBackupCodes(ctx, userID, codeHashes); err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}
	user.MFAEnabled = true
	if err := s.userRepo.Save(ctx, user); err != nil {
		return nil, err
	}
	return &MfaVerifySetupResponse{BackupCodes: backupCodes}, nil
}