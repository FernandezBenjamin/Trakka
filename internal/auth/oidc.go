package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// clockSkew is the tolerance applied when checking id_token exp/iat, to
// absorb minor clock drift between Trakka and the OIDC provider.
const clockSkew = 60 * time.Second

// jwksCacheTTL controls how long fetched JWKS keys are trusted before a
// routine refresh; an unknown kid always forces one immediate refetch
// regardless of this TTL, to handle key rotation promptly.
const jwksCacheTTL = time.Hour

// ProviderConfig is the subset of an OIDC discovery document Trakka needs.
type ProviderConfig struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// OIDCClient is a minimal, hand-rolled OIDC Authorization Code + PKCE
// client: discovery, authorization URL construction, code exchange, and
// RS256 id_token verification against the provider's JWKS. Deliberately
// stdlib-only (no coreos/go-oidc or golang.org/x/oauth2), to keep Trakka's
// only new dependency at bcrypt.
type OIDCClient struct {
	httpClient   *http.Client
	issuer       string
	clientID     string
	clientSecret string
	redirectURI  string

	provider *ProviderConfig

	mu     sync.RWMutex
	keys   map[string]*rsa.PublicKey
	keysAt time.Time
}

// NewOIDCClient runs OIDC discovery eagerly, so a broken OIDC configuration
// fails the process at startup rather than on a user's first login.
func NewOIDCClient(ctx context.Context, issuer, clientID, clientSecret, redirectURI string) (*OIDCClient, error) {
	c := &OIDCClient{
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		issuer:       issuer,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, fmt.Errorf("building discovery request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery document request returned %d", resp.StatusCode)
	}

	var provider ProviderConfig
	if err := json.NewDecoder(resp.Body).Decode(&provider); err != nil {
		return nil, fmt.Errorf("decoding discovery document: %w", err)
	}
	if provider.Issuer != issuer {
		return nil, fmt.Errorf("discovery document issuer %q does not match configured issuer %q", provider.Issuer, issuer)
	}
	if provider.AuthorizationEndpoint == "" || provider.TokenEndpoint == "" || provider.JWKSURI == "" {
		return nil, errors.New("discovery document is missing required endpoints")
	}
	c.provider = &provider

	return c, nil
}

// AuthorizationURL builds the URL to redirect the user to at the IdP.
func (c *OIDCClient) AuthorizationURL(state, nonce, codeChallenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.clientID},
		"redirect_uri":          {c.redirectURI},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.provider.AuthorizationEndpoint + "?" + q.Encode()
}

type tokenResponse struct {
	IDToken string `json:"id_token"`
}

// Exchange trades an authorization code for an id_token and returns its
// verified claims.
func (c *OIDCClient) Exchange(ctx context.Context, code, codeVerifier string) (*IDTokenClaims, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.redirectURI},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.provider.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if tr.IDToken == "" {
		return nil, errors.New("token response did not include an id_token")
	}

	return c.VerifyIDToken(ctx, tr.IDToken)
}

// VerifyIDToken verifies an id_token's RS256 signature against the
// provider's JWKS and validates its issuer, audience, and expiry. It does
// not check the nonce — the caller compares that against the value it
// stashed before starting the flow.
func (c *OIDCClient) VerifyIDToken(ctx context.Context, rawIDToken string) (*IDTokenClaims, error) {
	claims, err := verifyIDToken(rawIDToken, func(kid string) (*rsa.PublicKey, error) {
		return c.getKey(ctx, kid)
	})
	if err != nil {
		return nil, err
	}

	if claims.Issuer != c.issuer {
		return nil, fmt.Errorf("id_token issuer %q does not match expected %q", claims.Issuer, c.issuer)
	}
	if !containsString(claims.Audience, c.clientID) {
		return nil, errors.New("id_token audience does not include this client")
	}
	now := time.Now()
	if now.After(time.Unix(claims.Expiry, 0).Add(clockSkew)) {
		return nil, errors.New("id_token has expired")
	}
	if time.Unix(claims.IssuedAt, 0).After(now.Add(clockSkew)) {
		return nil, errors.New("id_token issued in the future")
	}

	return claims, nil
}

func (c *OIDCClient) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	fresh := time.Since(c.keysAt) < jwksCacheTTL
	c.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}

	if err := c.refreshKeys(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	key, ok = c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("no signing key found for kid %q", kid)
	}
	return key, nil
}

func (c *OIDCClient) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.provider.JWKSURI, nil)
	if err != nil {
		return fmt.Errorf("building JWKS request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var set jwks
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decoding JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}

	c.mu.Lock()
	c.keys = keys
	c.keysAt = time.Now()
	c.mu.Unlock()
	return nil
}
