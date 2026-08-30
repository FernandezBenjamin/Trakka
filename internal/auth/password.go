package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password for storage.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword reports whether plain matches hash.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// ErrPasswordTooLong is returned for a password bcrypt cannot hash.
var ErrPasswordTooLong = errors.New("password must be at most 72 bytes")

// maxPasswordBytes is bcrypt's own hard input limit. golang.org/x/crypto's
// GenerateFromPassword refuses anything longer outright (rather than
// silently truncating, as older implementations did), so without this check
// a user picking a long passphrase got an opaque "bad_request" redirect from
// the registration form with no indication of what was wrong. Checking it
// here turns that into a specific, actionable error — and documents the
// ceiling rather than leaving it as an accident of the hash function.
const maxPasswordBytes = 72

// ValidatePasswordStrength rejects passwords that are too short to be
// worth hashing, or longer than bcrypt can accept.
func ValidatePasswordStrength(plain string) error {
	if len(plain) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(plain) > maxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}
