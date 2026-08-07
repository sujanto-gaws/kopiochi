package application

import (
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
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
	audit          Auditor
}

// NewService creates a new identity service.
//
// A nil auditor is replaced with NopAuditor rather than kept: a nil interface
// would panic at exactly the moment a security event needed recording, turning
// a detected token theft into a 500 — the worst possible time to discover the
// wiring was incomplete.
func NewService(userRepo domain.UserRepository,
	passwordHasher domain.PasswordHasher,
	tokenIssuer domain.TokenIssuer,
	tokenStore domain.RefreshTokenStore,
	cfg Config,
	mfaService domain.MFAService,
	mfaStore domain.MFAStore,
	auditor Auditor) *Service {
	if auditor == nil {
		auditor = NopAuditor()
	}
	return &Service{
		userRepo:       userRepo,
		passwordHasher: passwordHasher,
		tokenIssuer:    tokenIssuer,
		tokenStore:     tokenStore,
		cfg:            cfg,
		mfaService:     mfaService,
		mfaStore:       mfaStore,
		audit:          auditor,
	}
}
