package auth

import (
	"context"
)

func (s *Service) VerifyMFA(ctx context.Context, mfaToken string, req MfaVerifyRequest) (*TokenResponse, error) {
	// Validate the temporary MFA token
	claims, err := s.tokenIssuer.Validate(mfaToken)
	if err != nil || claims.Scope != "mfa" {
		return nil, ErrInvalidMFAToken
	}
	user, err := s.userRepo.FindByID(ctx, claims.Subject)
	if err != nil {
		return nil, ErrInvalidMFAToken
	}
	// Verify code or backup code
	if req.Code != "" {
		if !s.mfaService.ValidateCode(user.MFASecret, req.Code) {
			return nil, ErrInvalidMFACode
		}
	} else if req.BackupCode != "" {
		found, err := s.mfaStore.FindAndUseBackupCode(ctx, user.ID.String(), req.BackupCode)
		if err != nil || !found {
			return nil, ErrInvalidMFACode
		}
	} else {
		return nil, ErrInvalidMFACode
	}

	return s.issueFullTokens(ctx, *user)
}
