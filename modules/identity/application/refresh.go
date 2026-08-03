package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// Refresh exchanges a refresh token for a new pair, rotating the old one away.
//
// Rotation on its own does not survive theft. Once a stolen token is used,
// both parties hold something that looks valid, and whoever refreshes second
// presents a token that has already been spent. Refusing just that one request
// leaves the other party — usually the attacker, who moves first — with a
// working session and nobody any the wiser.
//
// So a spent token is not treated as "invalid". It is treated as evidence, and
// the whole family descending from that login is revoked: both the victim and
// the thief are logged out, and the victim's next login is a real
// re-authentication. See docs/architectures/04-security/token-architecture.md.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	if refreshToken == "" {
		return nil, ErrRefreshTokenInvalid
	}
	hash := domain.HashToken(refreshToken)

	stored, err := s.tokenStore.FindValid(ctx, hash)
	if err != nil || stored == nil {
		// Not usable. Before rejecting, establish *why*: a token that never
		// existed is a bad request; one that exists and has been spent is a
		// security event.
		return nil, s.handleUnusableRefreshToken(ctx, hash)
	}

	user, err := s.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, ErrRefreshTokenInvalid
	}

	newPlain, err := generateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("random: %w", err)
	}

	next := domain.RefreshToken{
		UserID:    user.ID.String(),
		TokenHash: domain.HashToken(newPlain),
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
		// Same family: this token descends from the one being retired.
		FamilyID: stored.FamilyID,
	}

	// Atomic. Retiring the old token without storing the new one logs the user
	// out mid-session; storing the new one without retiring the old leaves two
	// live tokens in one family, which the reuse check would later report as a
	// theft that never happened.
	if err := s.tokenStore.Rotate(ctx, hash, next); err != nil {
		// Lost a race with a concurrent refresh of the same token: both reads
		// saw a usable token, only one UPDATE matched.
		if errors.Is(err, domain.ErrRefreshTokenAlreadyUsed) {
			return nil, s.revokeFamilyForReuse(ctx, stored)
		}
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	access, err := s.tokenIssuer.IssueAccessToken(*user, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("access token: %w", err)
	}
	idToken, _ := s.tokenIssuer.IssueIDToken(*user, s.cfg.ClientID)

	return &TokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
		RefreshToken: newPlain,
		IDToken:      idToken,
	}, nil
}

// handleUnusableRefreshToken decides whether an unusable token is merely
// invalid or is evidence of theft.
//
// What the client sees is deliberately identical either way. Telling an
// attacker "that one was recognised, and we have now locked the family" hands
// them confirmation that they held a real credential and tells them exactly
// when they were caught. The difference is recorded in the audit stream and in
// the database, where it is useful, and nowhere the caller can read it.
func (s *Service) handleUnusableRefreshToken(ctx context.Context, hash string) error {
	known, err := s.tokenStore.FindAny(ctx, hash)
	if err != nil || known == nil {
		// Never issued, or already swept. Nothing to revoke.
		return ErrRefreshTokenInvalid
	}

	if known.Used || known.Revoked {
		return s.revokeFamilyForReuse(ctx, known)
	}

	// Present, unused, unrevoked — so it is simply expired. An expired token
	// is the ordinary end of a session, not an attack.
	return ErrRefreshTokenInvalid
}

// revokeFamilyForReuse kills the whole chain and records the event.
//
// It always returns ErrRefreshTokenInvalid: the revocation is a side effect,
// and this path must be indistinguishable from an ordinary rejection.
func (s *Service) revokeFamilyForReuse(ctx context.Context, tok *domain.RefreshToken) error {
	revoked, err := s.tokenStore.RevokeFamily(ctx, tok.FamilyID)
	if err != nil {
		// A detection whose revocation did not land is the one case where the
		// attacker still holds a working session, so it is audited as such —
		// but it still must not become a 500 that tells the caller something
		// unusual just happened.
		s.audit.RefreshReuseDetected(ctx, tok.UserID, tok.FamilyID, 0, err)
		return ErrRefreshTokenInvalid
	}

	s.audit.RefreshReuseDetected(ctx, tok.UserID, tok.FamilyID, revoked, nil)
	return ErrRefreshTokenInvalid
}
