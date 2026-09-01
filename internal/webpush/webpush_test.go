package webpush

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

// The values below are RFC 8291 Appendix A's own worked "Encryption
// Example" — a fixed application-server key pair, a fixed receiver
// (User Agent) key pair and auth secret, and a fixed salt, all chosen by the
// RFC's authors specifically so an independent implementation can verify its
// output byte-for-byte against the RFC's own published ciphertext, rather
// than only round-tripping against itself. Fetched directly from
// https://www.rfc-editor.org/rfc/rfc8291.txt rather than retyped from
// memory. These are the RFC's own public example values, not any real VAPID
// identity or subscription this app has ever used — none of them are
// secrets, which is why the three high-entropy ones are each marked
// `gitleaks:allow` below rather than removed: they trip gitleaks'
// generic-api-key rule purely on their `...Key`/`...Secret` identifier names
// and entropy, and this test (see TestEncryptPayloadMatchesRFC8291Example's
// own doc comment) is the strongest correctness signal this package has
// short of a live push service, so it stays rather than being deleted to
// silence the scanner.
const (
	rfcPlaintextB64    = "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24"
	rfcASPublicKeyB64  = "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcASPrivateKeyB64 = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw" // gitleaks:allow -- RFC 8291 Appendix A's own published example key, not a real credential
	rfcUAPublicKeyB64  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcAuthSecretB64   = "BTBZMqHH6r4Tts7J_aSIgg" // gitleaks:allow -- RFC 8291 Appendix A's own published example value, not a real credential
	rfcSaltB64         = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcECDHSecretB64   = "kyrL1jIIOHEzg3sM2ZWRHDRB62YACZhhSlknJ672kSs" // gitleaks:allow -- an intermediate value derived from the RFC's own example, not a real credential
	rfcIKMB64          = "S4lYMb_L0FxCeq0WhDx813KgSYqU26kOyzWUdsXYyrg"
	rfcPRKB64          = "09_eUZGrsvxChDCGRCdkLiDXrReGOEVeSCdCcPBSJSc"
	rfcCEKB64          = "oIhVW04MRdy2XN9CiKLxTg"
	rfcNonceB64        = "4h_95klXJ5E_qnoN"
	rfcFinalPayloadB64 = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func mustDecodeB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := b64.DecodeString(s)
	if err != nil {
		t.Fatalf("decoding %q: %v", s, err)
	}
	return b
}

// TestEncryptPayloadMatchesRFC8291Example drives encryptPayloadWithParams
// with RFC 8291 Appendix A's exact fixed inputs and asserts every
// intermediate value (ECDH secret, IKM, PRK, CEK, nonce) and the final
// aes128gcm body match the RFC's own published values byte-for-byte. This
// is the primary correctness check for this package's payload encryption —
// a mismatch anywhere in the HKDF chain, the AES-GCM framing, or the
// aes128gcm header layout would show up here rather than only being
// discovered against a real push service later.
func TestEncryptPayloadMatchesRFC8291Example(t *testing.T) {
	plaintext := mustDecodeB64(t, rfcPlaintextB64)
	salt := mustDecodeB64(t, rfcSaltB64)
	asPrivScalar := mustDecodeB64(t, rfcASPrivateKeyB64)

	asPriv, err := ecdh.P256().NewPrivateKey(asPrivScalar)
	if err != nil {
		t.Fatalf("loading RFC application-server private key: %v", err)
	}
	if got, want := b64.EncodeToString(asPriv.PublicKey().Bytes()), rfcASPublicKeyB64; got != want {
		t.Fatalf("application-server public key derived from private key = %s, want %s (sanity check on the RFC's own key pair)", got, want)
	}

	// Recompute the intermediate values the same way encryptPayloadWithParams
	// does internally, so a failure points at exactly which derivation step
	// diverged from the RFC rather than just "the final bytes differ".
	subPub, err := decodeECDHPublicKey(rfcUAPublicKeyB64)
	if err != nil {
		t.Fatalf("decoding UA public key: %v", err)
	}
	ecdhSecret, err := asPriv.ECDH(subPub)
	if err != nil {
		t.Fatalf("computing ECDH secret: %v", err)
	}
	if got, want := b64.EncodeToString(ecdhSecret), rfcECDHSecretB64; got != want {
		t.Errorf("ECDH shared secret = %s, want %s", got, want)
	}

	authSecret := mustDecodeB64(t, rfcAuthSecretB64)
	asPub := asPriv.PublicKey().Bytes()
	uaPub := subPub.Bytes()
	keyInfo := append(append([]byte("WebPush: info\x00"), uaPub...), asPub...)
	prkKey := hkdfExtract(authSecret, ecdhSecret)
	ikm, err := hkdfExpand(prkKey, keyInfo, 32)
	if err != nil {
		t.Fatalf("deriving IKM: %v", err)
	}
	if got, want := b64.EncodeToString(ikm), rfcIKMB64; got != want {
		t.Errorf("IKM = %s, want %s", got, want)
	}

	prk := hkdfExtract(salt, ikm)
	if got, want := b64.EncodeToString(prk), rfcPRKB64; got != want {
		t.Errorf("PRK = %s, want %s", got, want)
	}

	cek, err := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	if err != nil {
		t.Fatalf("deriving CEK: %v", err)
	}
	if got, want := b64.EncodeToString(cek), rfcCEKB64; got != want {
		t.Errorf("CEK = %s, want %s", got, want)
	}

	nonce, err := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)
	if err != nil {
		t.Fatalf("deriving nonce: %v", err)
	}
	if got, want := b64.EncodeToString(nonce), rfcNonceB64; got != want {
		t.Errorf("nonce = %s, want %s", got, want)
	}

	got, err := encryptPayloadWithParams(rfcUAPublicKeyB64, rfcAuthSecretB64, plaintext, salt, asPriv)
	if err != nil {
		t.Fatalf("encryptPayloadWithParams: %v", err)
	}
	want := mustDecodeB64(t, rfcFinalPayloadB64)
	if !bytes.Equal(got, want) {
		t.Fatalf("encrypted payload mismatch\n got:  %s\n want: %s", b64.EncodeToString(got), rfcFinalPayloadB64)
	}
}

// TestEncryptPayloadRejectsBadInputs covers the validation paths
// encryptPayload's real (non-test-fixed) entry point relies on.
func TestEncryptPayloadRejectsBadInputs(t *testing.T) {
	if _, err := encryptPayload("not-base64!!!", rfcAuthSecretB64, []byte("hi")); err == nil {
		t.Error("expected an error for an unparseable p256dh key")
	}
	if _, err := encryptPayload(rfcUAPublicKeyB64, b64.EncodeToString([]byte("too-short")), []byte("hi")); err == nil {
		t.Error("expected an error for an auth secret that isn't 16 bytes")
	}
	if _, err := encryptPayload(b64.EncodeToString([]byte("not-a-valid-ec-point-at-all-not-a-valid-ec-point")), rfcAuthSecretB64, []byte("hi")); err == nil {
		t.Error("expected an error for a p256dh that isn't a valid P-256 point")
	}
}

// TestEncryptPayloadRandomizedRoundTrip encrypts through the real
// (randomized) entry point twice with the same inputs and confirms the two
// outputs differ — salt and the ephemeral key pair must be fresh per call,
// never reused, or an observer could correlate two pushes to the same
// subscription.
func TestEncryptPayloadRandomizedRoundTrip(t *testing.T) {
	first, err := encryptPayload(rfcUAPublicKeyB64, rfcAuthSecretB64, []byte("hello"))
	if err != nil {
		t.Fatalf("first encryptPayload: %v", err)
	}
	second, err := encryptPayload(rfcUAPublicKeyB64, rfcAuthSecretB64, []byte("hello"))
	if err != nil {
		t.Fatalf("second encryptPayload: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Error("two encryptions of the same plaintext produced identical output — salt/ephemeral key must be fresh per call")
	}
	// Both must still carry the fixed 16-byte-salt + 4-byte-rs + 1-byte-idlen
	// + 65-byte-key header shape regardless of the random values inside it.
	for _, out := range [][]byte{first, second} {
		if len(out) < 16+4+1+65 {
			t.Errorf("encrypted payload too short for the aes128gcm header: %d bytes", len(out))
		}
		if idlen := out[20]; idlen != 65 {
			t.Errorf("idlen byte = %d, want 65 (an uncompressed P-256 point)", idlen)
		}
	}
}

// ---------------------------------------------------------------------------
// VAPID JWT signing
// ---------------------------------------------------------------------------

// vapidPublicKeyFromB64 is encrypt/vapid.go's vapidPrivateKeyFromB64 in
// reverse, kept test-only: production code never needs to turn
// VAPID_PUBLIC_KEY back into an *ecdsa.PublicKey (Send just forwards the
// base64 string to the Authorization header verbatim, and the push service
// on the other end is what actually verifies the signature) — only this
// test does, to confirm signVAPIDJWT produces a signature that verifies
// against the public half of the same key pair. Uses ecdsa.ParseUncompressedPublicKey
// rather than reconstructing the deprecated X/Y big.Int fields by hand.
func vapidPublicKeyFromB64(t *testing.T, raw string) *ecdsa.PublicKey {
	t.Helper()
	point := mustDecodeB64(t, raw)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
	if err != nil {
		t.Fatalf("parsing public key point: %v", err)
	}
	return pub
}

func TestSignVAPIDJWTRoundTrips(t *testing.T) {
	keys, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	priv, err := vapidPrivateKeyFromB64(keys.PrivateKeyB64)
	if err != nil {
		t.Fatalf("vapidPrivateKeyFromB64: %v", err)
	}

	jwt, err := signVAPIDJWT(priv, "https://push.example.com", "mailto:ops@example.com", time.Hour)
	if err != nil {
		t.Fatalf("signVAPIDJWT: %v", err)
	}

	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d segments, want 3", len(parts))
	}

	headerJSON := mustDecodeB64(t, parts[0])
	var header struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshaling header: %v", err)
	}
	if header.Typ != "JWT" || header.Alg != "ES256" {
		t.Errorf("header = %+v, want typ=JWT alg=ES256", header)
	}

	claimsJSON := mustDecodeB64(t, parts[1])
	var claims vapidClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshaling claims: %v", err)
	}
	if claims.Aud != "https://push.example.com" {
		t.Errorf("aud = %q, want https://push.example.com", claims.Aud)
	}
	if claims.Sub != "mailto:ops@example.com" {
		t.Errorf("sub = %q, want mailto:ops@example.com", claims.Sub)
	}
	if claims.Exp <= time.Now().Unix() || claims.Exp > time.Now().Add(2*time.Hour).Unix() {
		t.Errorf("exp = %d, want roughly now+1h", claims.Exp)
	}

	sig := mustDecodeB64(t, parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, want 64 (raw r||s, not ASN.1 DER)", len(sig))
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	pub := vapidPublicKeyFromB64(t, keys.PublicKeyB64)
	if !ecdsa.Verify(pub, digest[:], r, s) {
		t.Error("signature does not verify against the key pair's own public key")
	}
}

func TestSignVAPIDJWTRejectsUnparseablePrivateKey(t *testing.T) {
	if _, err := vapidPrivateKeyFromB64("not-valid-base64!!!"); err == nil {
		t.Error("expected an error for an unparseable VAPID private key")
	}
}
