package auth

import (
	domain "github.com/sujanto-gaws/kopiochi/internal/domain/auth"
)

// Service implements the user application service
type Service struct {
	userRepo       domain.UserRepository
	passwordHasher domain.PasswordHasher
	tokenIssuer    domain.TokenIssuer
	tokenStore     domain.RefreshTokenStore
	cfg            Config
	mfaService     domain.MFAService
	mfaStore       domain.MFAStore
}

// NewService creates a new user service
func NewService(userRepo domain.UserRepository,
	passwordHasher domain.PasswordHasher,
	tokenIssuer domain.TokenIssuer,
	tokenStore domain.RefreshTokenStore,
	cfg Config,
	mfaService domain.MFAService,
	mfaStore domain.MFAStore) *Service {
	return &Service{
		userRepo:       userRepo,
		passwordHasher: passwordHasher,
		tokenIssuer:    tokenIssuer,
		tokenStore:     tokenStore,
		cfg:            cfg,
		mfaService:     mfaService,
		mfaStore:       mfaStore,
	}
}
