package webpush

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"
)

// b64 is the URL-safe, unpadded base64 encoding every key, JWT segment, and
// push-service-facing value in this package uses — the encoding the Web
// Push/VAPID RFCs and every browser's Push API both use throughout.
var b64 = base64.RawURLEncoding

// vapidJWTTTL is comfortably under RFC 8292's 24-hour maximum for a VAPID
// JWT's own expiry. A fresh JWT is signed on every Send call — this app's
// traffic is far too low for caching one across requests to matter — so
// this only bounds how long a signature stays valid in flight, never how
// often one is issued.
const vapidJWTTTL = 12 * time.Hour

// VAPIDKeyPair holds an application server's VAPID identity: a P-256 key
// pair used to sign the JWT sent with every push request.
// PublicKeyB64 is also what a browser's PushManager.subscribe() expects as
// applicationServerKey — a raw, uncompressed 65-byte EC point,
// base64url-encoded — so the same value read from config
// (VAPID_PUBLIC_KEY) is handed straight to the frontend by
// GET /api/v1/push/vapid-public-key (internal/handlers/push.go) with no
// re-encoding.
type VAPIDKeyPair struct {
	PublicKeyB64  string
	PrivateKeyB64 string
}

// GenerateVAPIDKeys creates a fresh P-256 key pair, encoded the way
// VAPID_PUBLIC_KEY/VAPID_PRIVATE_KEY expect. Exposed for the
// `trakka -generate-vapid-keys` CLI flag (cmd/server/main.go) — a one-time
// setup step for an operator, never called at request time.
func GenerateVAPIDKeys() (VAPIDKeyPair, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return VAPIDKeyPair{}, fmt.Errorf("generating VAPID key pair: %w", err)
	}
	return VAPIDKeyPair{
		PublicKeyB64:  b64.EncodeToString(priv.PublicKey().Bytes()),
		PrivateKeyB64: b64.EncodeToString(priv.Bytes()),
	}, nil
}

// vapidPrivateKeyFromB64 reconstructs a signing-capable *ecdsa.PrivateKey
// from a base64url-encoded raw P-256 scalar (VAPID_PRIVATE_KEY, as produced
// by GenerateVAPIDKeys above). It goes through crypto/ecdh first purely to
// validate the scalar is in the curve's valid range using the same check a
// browser-side ECDH import would apply, then hands the same raw bytes to
// ecdsa.ParseRawPrivateKey — the two accept an identical raw-scalar encoding
// for NIST curves — which derives the public point internally rather than
// this package reconstructing X/Y/D by hand via the deprecated big.Int-based
// ecdsa.PrivateKey/PublicKey fields (crypto/elliptic's point-arithmetic
// helpers like Marshal/ScalarBaseMult are deprecated the same way).
func vapidPrivateKeyFromB64(raw string) (*ecdsa.PrivateKey, error) {
	scalar, err := b64.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding VAPID private key: %w", err)
	}
	if _, err := ecdh.P256().NewPrivateKey(scalar); err != nil {
		return nil, fmt.Errorf("parsing VAPID private key: %w", err)
	}
	priv, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), scalar)
	if err != nil {
		return nil, fmt.Errorf("parsing VAPID private key: %w", err)
	}
	return priv, nil
}

// vapidClaims is the VAPID JWT's payload (RFC 8292 §2): aud is the push
// endpoint's own origin (scheme://host — never the full endpoint URL, which
// would leak the subscription's own unique path to a claim the push service
// itself already knows how to route), exp bounds how long the signature is
// valid, and sub identifies the sending application (a mailto: or https:
// URI, VAPID_SUBJECT).
type vapidClaims struct {
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
	Sub string `json:"sub"`
}

// signVAPIDJWT builds and signs the ES256 JWT VAPID requires as proof of
// application-server identity (RFC 8292): header {"typ":"JWT","alg":"ES256"},
// claims per vapidClaims above.
func signVAPIDJWT(priv *ecdsa.PrivateKey, audience, subject string, ttl time.Duration) (string, error) {
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))

	claims, err := json.Marshal(vapidClaims{Aud: audience, Exp: time.Now().Add(ttl).Unix(), Sub: subject})
	if err != nil {
		return "", fmt.Errorf("marshaling VAPID claims: %w", err)
	}
	payload := b64.EncodeToString(claims)

	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing VAPID JWT: %w", err)
	}

	return signingInput + "." + b64.EncodeToString(rawECDSASignature(r, s)), nil
}

// rawECDSASignature encodes an ECDSA signature the way JWS ES256 requires
// (RFC 7518 §3.4): r and s each left-padded to 32 bytes (P-256's coordinate
// size) and concatenated — never the ASN.1 DER sequence crypto/ecdsa.Sign's
// r/s pair would otherwise be wrapped in for, e.g., an X.509 certificate.
func rawECDSASignature(r, s *big.Int) []byte {
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:64])
	return out
}
