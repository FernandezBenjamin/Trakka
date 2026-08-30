package auth

import (
	"context"
	"errors"
	"strings"

	"trakka/internal/db"
	"trakka/internal/models"
	"trakka/internal/validate"
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
//
// allowProvisioning carries the instance's "registration open" setting. An
// identity that already has a local account always logs in regardless of it;
// only creating a brand new account is gated. Before this parameter existed,
// closing registration only closed the local email/password form and left
// OIDC auto-provisioning wide open, so anyone with an account at the
// configured IdP could still obtain a Trakka account on an instance whose
// administrator believed registration was shut.
func (s *Service) LoginOrProvisionOIDCUser(ctx context.Context, claims *IDTokenClaims, allowProvisioning bool) (*models.User, error) {
	existing, err := s.DB.GetUserByOIDCSubject(ctx, claims.Issuer, claims.Subject)
	if err == nil {
		return &existing.User, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	if !allowProvisioning {
		return nil, ErrRegistrationClosed
	}

	if claims.Email != "" {
		if _, err := s.DB.GetUserByEmail(ctx, claims.Email); err == nil {
			return nil, ErrOIDCEmailConflict
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
	}

	// The email and name claims come from the IdP, not from Trakka: bound
	// them the same way the local registration path bounds its own inputs, so
	// a hostile or misconfigured provider cannot persist an unbounded string.
	email := strings.ToLower(validate.Text(claims.Email))
	if !validate.MaxLen(email, validate.MaxEmailLen) {
		return nil, errors.New("oidc email claim is too long")
	}
	displayName := validate.Text(claims.Name)
	if !validate.MaxLen(displayName, validate.MaxDisplayNameLen) {
		return nil, errors.New("oidc name claim is too long")
	}

	subject, issuer := claims.Subject, claims.Issuer
	user, err := s.DB.CreateUser(ctx, email, nil, &subject, &issuer, displayName)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.CreateHouseWithOwner(ctx, DefaultHouseName, user.ID); err != nil {
		return nil, err
	}
	return user, nil
}
