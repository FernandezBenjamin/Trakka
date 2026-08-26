package auth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// IDTokenClaims is the subset of OIDC id_token claims Trakka cares about.
type IDTokenClaims struct {
	Issuer        string
	Subject       string
	Audience      []string
	Expiry        int64
	IssuedAt      int64
	Nonce         string
	Email         string
	EmailVerified bool
	Name          string
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// audience accepts the OIDC spec's two legal shapes for "aud": a single
// string, or an array of strings.
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(data, &multi); err != nil {
		return err
	}
	*s = multi
	return nil
}

type rawClaims struct {
	Issuer        string        `json:"iss"`
	Subject       string        `json:"sub"`
	Audience      stringOrSlice `json:"aud"`
	Expiry        int64         `json:"exp"`
	IssuedAt      int64         `json:"iat"`
	Nonce         string        `json:"nonce"`
	Email         string        `json:"email"`
	EmailVerified bool          `json:"email_verified"`
	Name          string        `json:"name"`
}

// verifyIDToken performs hand-rolled RS256 JWT verification: it rejects
// anything but RS256 before ever looking up a key (alg-confusion defense),
// verifies the signature against the provider's JWKS, and only then trusts
// the claims. getKey is the client's JWKS lookup (fetch+cache).
func verifyIDToken(rawIDToken string, getKey func(kid string) (*rsa.PublicKey, error)) (*IDTokenClaims, error) {
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed id_token")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decoding id_token header: %w", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("parsing id_token header: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported id_token signing algorithm %q", header.Alg)
	}

	pubKey, err := getKey(header.Kid)
	if err != nil {
		return nil, fmt.Errorf("resolving signing key: %w", err)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decoding id_token signature: %w", err)
	}
	signingInput := parts[0] + "." + parts[1]
	hashed := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], sig); err != nil {
		return nil, fmt.Errorf("id_token signature verification failed: %w", err)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding id_token payload: %w", err)
	}
	var raw rawClaims
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return nil, fmt.Errorf("parsing id_token claims: %w", err)
	}

	return &IDTokenClaims{
		Issuer:        raw.Issuer,
		Subject:       raw.Subject,
		Audience:      raw.Audience,
		Expiry:        raw.Expiry,
		IssuedAt:      raw.IssuedAt,
		Nonce:         raw.Nonce,
		Email:         raw.Email,
		EmailVerified: raw.EmailVerified,
		Name:          raw.Name,
	}, nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
