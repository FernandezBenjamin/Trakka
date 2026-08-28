package auth

import (
	"context"
	"errors"

	"trakka/internal/db"
	"trakka/internal/models"
)

// DefaultHouseName is used for the personal house auto-created for a user
// the first time they register or log in via OIDC.
const DefaultHouseName = "Ma Maison"

// LoginOrProvisionOIDCUser resolves an OIDC identity to a local user,
// creating one (with a default personal house) on first login. If the
// claimed email already belongs to a different account, this returns
// ErrOIDCEmailConflict rather than silently linking the accounts — the
// OIDC provider's email claim isn't guaranteed to be verified, so
// auto-linking would be an account-takeover risk.
func (s *Service) LoginOrProvisionOIDCUser(ctx context.Context, claims *IDTokenClaims) (*models.User, error) {
	existing, err := s.DB.GetUserByOIDCSubject(ctx, claims.Issuer, claims.Subject)
	if err == nil {
		return &existing.User, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	if claims.Email != "" {
		if _, err := s.DB.GetUserByEmail(ctx, claims.Email); err == nil {
			return nil, ErrOIDCEmailConflict
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
	}

	subject, issuer := claims.Subject, claims.Issuer
	user, err := s.DB.CreateUser(ctx, claims.Email, nil, &subject, &issuer, claims.Name)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.CreateHouseWithOwner(ctx, DefaultHouseName, user.ID); err != nil {
		return nil, err
	}
	return user, nil
}
