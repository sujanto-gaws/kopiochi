package auth

import (
	"context"
)

func (s *Service) SetupMFA(ctx context.Context, userID string) (*MfaSetupResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	secret, qr, err := s.mfaService.GenerateSecret(user.Email)
	if err != nil {
		return nil, err
	}
	// Temporarily store the secret without enabling MFA yet.
	user.MFASecret = secret
	// Do NOT set MFAEnabled = true now.
	if err := s.userRepo.Save(ctx, user); err != nil {
		return nil, err
	}
	return &MfaSetupResponse{
		Secret:    secret,
		QRCodeURL: qr,
	}, nil
}