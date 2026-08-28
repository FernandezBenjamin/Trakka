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

// ValidatePasswordStrength rejects passwords that are too short to be
// worth hashing.
func ValidatePasswordStrength(plain string) error {
	if len(plain) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}
