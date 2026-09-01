// Package webpush implements just enough of Web Push (RFC 8030), VAPID
// application-server identification (RFC 8292), and the aes128gcm message
// encryption a push payload is carried in (RFC 8291) to deliver a
// notification to a subscription a browser handed this app via the Push
// API. It is hand-rolled on top of the standard library's crypto/ecdh,
// crypto/ecdsa, crypto/aes and crypto/cipher — no third-party Web Push
// library — per this project's "standard library wherever possible"
// convention (see CLAUDE.md); the same reasoning already justified
// hand-rolling this app's OIDC/JWT client (internal/auth).
package webpush

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// pushTTL is the Web Push TTL (RFC 8030 §5.2) attached to every push this
// package sends: how long the push service should hold the message for a
// device that's currently offline. A day is generous for a household list
// update or an upcoming-task reminder — neither is useful long after the
// fact — while still covering an overnight device restart.
const pushTTL = 24 * time.Hour

// requestTimeout bounds both the overall push HTTP request (httpClient.Timeout,
// below) and the dial itself (safeDialContext, ssrf.go) — kept as one named
// constant, rather than reading httpClient.Timeout from within
// safeDialContext, specifically to avoid an initialization cycle between the
// two package-level vars.
const requestTimeout = 10 * time.Second

// httpClient's Transport.DialContext is this package's own SSRF guard (see
// ssrf.go) — a subscription's Endpoint ultimately came from whatever a
// caller POSTed to /api/v1/push/subscribe (see internal/handlers/push.go),
// so it is exactly the kind of user-influenced URL CLAUDE.md's SSRF rule
// requires guarding: without this, it would let an authenticated caller
// make this server issue requests to internal services, loopback, or a
// cloud metadata endpoint. Built once as a package-level var for the same
// reason internal/scraper's own httpClient is.
var httpClient = &http.Client{
	Timeout:   requestTimeout,
	Transport: &http.Transport{DialContext: safeDialContext},
}

// Subscription is the minimal shape Send needs to deliver one push message.
// models.PushSubscription (internal/models) carries the same three fields
// plus the bookkeeping (owning user, timestamps) this package has no use
// for — handlers translate between the two.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// ErrSubscriptionGone is returned by Send when the push service reports the
// subscription no longer exists (410 Gone) or was never valid to begin with
// (404 Not Found) — RFC 8030 §7.3's documented signal that the caller
// should stop sending to it. Every other non-2xx response is returned as a
// plain error instead, since it may well be transient (rate limiting, a
// push service outage) — callers must not treat that the same as a
// confirmed-dead subscription; see sendToUsers in
// internal/handlers/push.go, which deletes the stored row only on this
// specific error.
var ErrSubscriptionGone = errors.New("push subscription no longer valid")

// Send delivers payload (arbitrary JSON — see internal/handlers/push.go for
// the {"title", "body", "url"} shape this app actually sends) to sub,
// encrypted per RFC 8291 and VAPID-signed with keys/subject. Web Push has no
// meaningful "send in the clear" option once a payload is involved, and
// every subscription this app stores always carries the p256dh/auth keys a
// browser's PushSubscriptionJSON always includes, so encryption is not
// optional here.
func Send(ctx context.Context, sub Subscription, keys VAPIDKeyPair, subject string, payload []byte) error {
	endpoint, err := url.Parse(sub.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		// Every real push service endpoint is https — this mirrors
		// internal/validate.URL's "already validated at the boundary, but
		// re-check anyway as defense in depth" pattern internal/scraper's
		// own FetchProductInfo follows for the same reason.
		return fmt.Errorf("invalid push endpoint %q", sub.Endpoint)
	}

	priv, err := vapidPrivateKeyFromB64(keys.PrivateKeyB64)
	if err != nil {
		return fmt.Errorf("loading VAPID private key: %w", err)
	}
	audience := endpoint.Scheme + "://" + endpoint.Host
	jwt, err := signVAPIDJWT(priv, audience, subject, vapidJWTTTL)
	if err != nil {
		return fmt.Errorf("signing VAPID JWT: %w", err)
	}

	body, err := encryptPayload(sub.P256dh, sub.Auth, payload)
	if err != nil {
		return fmt.Errorf("encrypting push payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(pushTTL.Seconds())))
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, keys.PublicKeyB64))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return ErrSubscriptionGone
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("push service responded %d", resp.StatusCode)
	}
	return nil
}
