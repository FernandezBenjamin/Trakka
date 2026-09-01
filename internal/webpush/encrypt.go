package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// recordSize is the "rs" field of the aes128gcm header (RFC 8188 §2): the
// maximum size of one content-coding record. Every payload this app ever
// sends (a short JSON notification body) fits in a single record, so this
// is simply large enough to never be a constraint, not a value tuned for
// anything.
const recordSize = 4096

// hkdfExtract and hkdfExpand implement RFC 5869's HKDF directly on top of
// crypto/hmac + crypto/sha256, rather than depending on exactly which Go
// toolchain version first shipped a dedicated crypto/hkdf package — HKDF is
// short enough (two HMAC-based steps) that hand-rolling it here removes that
// dependency entirely, the same "stdlib primitives, hand-rolled
// construction" approach already used for this app's OIDC/JWT client
// (internal/auth). Verified against RFC 8291 Appendix A's own worked
// example in encrypt_test.go, including these two intermediate values.
func hkdfExtract(salt, ikm []byte) []byte {
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

func hkdfExpand(prk, info []byte, length int) ([]byte, error) {
	if length > 255*sha256.Size {
		return nil, fmt.Errorf("hkdf: requested length %d too large", length)
	}
	out := make([]byte, 0, length)
	var prev []byte
	for i := byte(1); len(out) < length; i++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(prev)
		mac.Write(info)
		mac.Write([]byte{i})
		prev = mac.Sum(nil)
		out = append(out, prev...)
	}
	return out[:length], nil
}

// encryptPayload implements RFC 8291's Web Push message encryption (the
// "aes128gcm" HTTP content coding, RFC 8188, with the key derivation RFC
// 8291 layers on top of it) for a single-record message. subP256dhB64 and
// subAuthB64 are the subscription's own "p256dh"/"auth" keys exactly as a
// browser's PushSubscriptionJSON reports them (base64url). Returns the
// complete request body to send as-is — a 16-byte salt, the 4-byte record
// size, this message's own ephemeral public key, and the ciphertext, per
// RFC 8188 §2's header layout — with Content-Encoding: aes128gcm.
func encryptPayload(subP256dhB64, subAuthB64 string, plaintext []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	asPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ephemeral ECDH key: %w", err)
	}
	return encryptPayloadWithParams(subP256dhB64, subAuthB64, plaintext, salt, asPriv)
}

// encryptPayloadWithParams is encryptPayload's actual implementation, with
// the salt and this message's own ephemeral ECDH key pair taken as
// parameters rather than generated internally — this is what lets
// encrypt_test.go drive it with RFC 8291 Appendix A's exact fixed salt and
// application-server key pair and assert the output matches the RFC's own
// published ciphertext byte-for-byte, rather than only round-tripping
// against itself.
func encryptPayloadWithParams(subP256dhB64, subAuthB64 string, plaintext, salt []byte, asPriv *ecdh.PrivateKey) ([]byte, error) {
	subPub, err := decodeECDHPublicKey(subP256dhB64)
	if err != nil {
		return nil, fmt.Errorf("decoding subscription p256dh: %w", err)
	}
	authSecret, err := b64.DecodeString(subAuthB64)
	if err != nil {
		return nil, fmt.Errorf("decoding subscription auth secret: %w", err)
	}
	if len(authSecret) != 16 {
		return nil, errors.New("subscription auth secret must be 16 bytes")
	}

	asPub := asPriv.PublicKey().Bytes()
	uaPub := subPub.Bytes()

	ecdhSecret, err := asPriv.ECDH(subPub)
	if err != nil {
		return nil, fmt.Errorf("computing ECDH shared secret: %w", err)
	}

	// RFC 8291 §3.4: derive a pseudo-random key from the ECDH secret,
	// salted (in the HKDF sense) by the subscription's own auth secret,
	// then bind it to both parties' public keys via the "info" string —
	// this authenticates the key exchange against the specific
	// subscription (auth_secret) and the specific sender/receiver key pair
	// (keyInfo), not just "some valid ECDH result".
	keyInfo := make([]byte, 0, len(uaPub)+len(asPub)+14)
	keyInfo = append(keyInfo, "WebPush: info\x00"...)
	keyInfo = append(keyInfo, uaPub...)
	keyInfo = append(keyInfo, asPub...)
	prkKey := hkdfExtract(authSecret, ecdhSecret)
	ikm, err := hkdfExpand(prkKey, keyInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("deriving IKM: %w", err)
	}

	prk := hkdfExtract(salt, ikm)
	cek, err := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	if err != nil {
		return nil, fmt.Errorf("deriving CEK: %w", err)
	}
	nonce, err := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)
	if err != nil {
		return nil, fmt.Errorf("deriving nonce: %w", err)
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	// RFC 8188 §2's padding delimiter: 0x02 marks this record as the last
	// (and, here, only) one. No further padding is added beyond it — the
	// delimiter alone is sufficient for a single-record message.
	padded := make([]byte, 0, len(plaintext)+1)
	padded = append(padded, plaintext...)
	padded = append(padded, 0x02)
	ciphertext := gcm.Seal(nil, nonce, padded, nil)

	header := make([]byte, 0, 16+4+1+len(asPub)+len(ciphertext))
	header = append(header, salt...)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], recordSize)
	header = append(header, rs[:]...)
	header = append(header, byte(len(asPub))) // #nosec G115 -- asPub is asPriv.PublicKey().Bytes() from an internally-generated ecdh.P256() key, always exactly 65 bytes; never user-controlled and can never approach 256
	header = append(header, asPub...)
	return append(header, ciphertext...), nil
}

func decodeECDHPublicKey(raw string) (*ecdh.PublicKey, error) {
	b, err := b64.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding EC public key: %w", err)
	}
	pub, err := ecdh.P256().NewPublicKey(b)
	if err != nil {
		return nil, fmt.Errorf("parsing EC public key: %w", err)
	}
	return pub, nil
}
