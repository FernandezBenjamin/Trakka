package auth

import "errors"

// ErrInvalidCredentials is returned by Authenticate when the email/password
// combination doesn't match a local account, or the account has no
// password (OIDC-only). Kept generic on purpose: it must never reveal
// whether the email exists.
var ErrInvalidCredentials = errors.New("invalid email or password")

// ErrOIDCEmailConflict is returned by LoginOrProvisionOIDCUser when the
// OIDC identity's email already belongs to a different account (local or a
// different OIDC issuer). Rejected rather than auto-linked, since the OIDC
// provider's email claim isn't guaranteed to be verified.
var ErrOIDCEmailConflict = errors.New("an account with this email already exists")
