package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (s *Service) VerifyMFASetup(ctx context.Context, userID string, code string) (*MfaVerifySetupResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Refuse before validating, not because ValidateCode would accept an empty
	// secret — since E10 it will not — but because two independent reasons to
	// refuse are what makes this not depend on the other one staying correct.
	//
	// This is the reachable half of E10. SetupMFA is what stores the secret, and
	// nothing here required it to have run: a caller who posts to
	// /auth/mfa/setup/verify WITHOUT calling /auth/mfa/setup arrives with an
	// empty secret, and the code derived from the empty secret is public. That
	// path ended four lines below at `user.MFAEnabled = true` — an account
	// advertising a second factor that anybody can compute.
	if user.MFASecret == "" {
		return nil, ErrMFANotStarted
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
		code := hex.EncodeToString(b)[:8] // 8-digit hex code
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
	// After Save succeeds, not before: reporting an enablement that failed to
	// persist would notify about a second factor the account does not
	// actually have yet.
	s.notifier.MFAEnabled(ctx, userID, time.Now())
	return &MfaVerifySetupResponse{BackupCodes: backupCodes}, nil
}
