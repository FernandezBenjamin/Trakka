// Package scraper does a best-effort lookup of a listed price on a
// user-supplied product page, for the automatic-price-fill feature: given
// an item's URL, FetchPrice downloads the page and looks for a price in
// OpenGraph/Schema.org metadata. Every failure mode (network error,
// timeout, blocked host, no recognizable price markup) is reported the
// same way — a non-nil error — and callers are expected to treat that as
// "no price found" rather than a fatal condition; nothing here ever
// prevents an item from being created or updated.
package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	fetchTimeout    = 8 * time.Second
	maxResponseSize = 2 << 20 // 2 MiB is generous for a page's <head> + markup
	userAgent       = "TrakkaBot/1.0 (+price-lookup; https://github.com/)"
	maxRedirects    = 5
)

// httpClient is shared across calls: its Transport's DialContext is what
// enforces the SSRF guard below, and building that once (rather than per
// request) is the whole reason this is a package-level var.
var httpClient = &http.Client{
	Timeout: fetchTimeout,
	Transport: &http.Transport{
		DialContext: safeDialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// safeDialContext is a net.Dialer.DialContext replacement that resolves
// the target host itself and only ever connects to a public IP address —
// this is Trakka's SSRF defense for a feature that, by design, makes the
// server fetch an arbitrary URL a user typed in (an item's `url` field).
// Without this, that field would be a way to make the server issue
// requests to internal services, loopback, or a cloud metadata endpoint
// (e.g. 169.254.169.254). Resolving and validating here — then dialing the
// checked IP literal directly, rather than the hostname — also closes the
// DNS-rebinding TOCTOU gap a "resolve, check, then let the dialer
// re-resolve" approach would leave open. TLS (when the URL is https)
// still verifies the certificate against the original hostname, since
// net/http wraps whatever raw connection this returns using the request's
// original host for SNI/verification, not the dialed IP.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("splitting address %q: %w", addr, err)
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", host, err)
	}

	dialer := &net.Dialer{Timeout: fetchTimeout}
	var lastErr error
	for _, ip := range ips {
		if !isPublicIP(ip) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no public IP address for host %q", host)
	}
	return nil, lastErr
}

// isPublicIP reports whether ip is safe for this server to connect to on a
// user's behalf: not loopback, not link-local (this also covers the
// 169.254.169.254 cloud metadata address), not a private/ULA range, not a
// multicast or unspecified address.
func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsMulticast()
}

// ProductInfo is what FetchProductInfo manages to extract from a product
// page — either field may be the zero value if that particular piece
// wasn't found, but at least one is always non-zero when err is nil.
type ProductInfo struct {
	Price *float64
	// ImageURL, when non-empty, is always an absolute http(s) URL — see
	// resolveImageURL — safe to persist and to render as an <img src>.
	ImageURL string
}

// FetchProductInfo downloads rawURL and attempts to extract a listed price
// and a product image from its HTML, in a single fetch (both come from the
// same page, so there is no reason to request it twice). rawURL is expected
// to have already passed internal/validate.URL; FetchProductInfo re-checks
// the scheme anyway as defense in depth against being called with something
// else by mistake. A non-nil error covers every failure mode uniformly
// (network, timeout, blocked host, unrecognized markup, or the page simply
// carrying neither) — callers should treat it as "nothing found", not as an
// operation that needs to be retried or surfaced to the end user.
func FetchProductInfo(ctx context.Context, rawURL string) (*ProductInfo, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("unsupported url scheme %q", parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: unexpected status %d", rawURL, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "html") {
		return nil, fmt.Errorf("fetching %s: unexpected content-type %q", rawURL, ct)
	}

	info, err := extractProductInfo(io.LimitReader(resp.Body, maxResponseSize), parsed)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rawURL, err)
	}
	if info.Price == nil && info.ImageURL == "" {
		return nil, fmt.Errorf("no price or image found on %s", rawURL)
	}
	return info, nil
}

// extractProductInfo walks the parsed HTML tree once, looking for both a
// price and a product image. Price is resolved via three mechanisms, in
// priority order: OpenGraph/Schema.org meta tags (most structured and
// explicit when present), Schema.org JSON-LD blocks, then classic
// microdata (itemprop="price"). Image is resolved similarly: OpenGraph
// (og:image), then Schema.org JSON-LD ("image"), then the Twitter Card
// meta tag (twitter:image) — the same priority order the caller asked for
// price, kept for consistency. It returns a zero-value field (never an
// error) for anything not found, since that's an expected outcome, not a
// failure; pageURL is used to resolve a relative image URL to absolute.
func extractProductInfo(r io.Reader, pageURL *url.URL) (*ProductInfo, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing html: %w", err)
	}

	var ogPrice string
	var ogImage string
	var twitterImage string
	var ldjsonBlocks []string
	var microdataPrice string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				if ogPrice == "" {
					if p, ok := metaPrice(n); ok {
						ogPrice = p
					}
				}
				if ogImage == "" {
					if v, ok := metaContent(n, "property", "og:image"); ok {
						ogImage = v
					}
				}
				if twitterImage == "" {
					if v, ok := metaContent(n, "name", "twitter:image"); ok {
						twitterImage = v
					}
				}
			case "script":
				if isLDJSON(n) && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					ldjsonBlocks = append(ldjsonBlocks, n.FirstChild.Data)
				}
			default:
				if microdataPrice == "" && hasItemPropPrice(n) {
					microdataPrice = microdataValue(n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	info := &ProductInfo{}

	if ogPrice != "" {
		if p, ok := parsePriceString(ogPrice); ok {
			info.Price = &p
		}
	}
	if info.Price == nil {
		for _, block := range ldjsonBlocks {
			if p, ok := findJSONPrice(block); ok {
				info.Price = &p
				break
			}
		}
	}
	if info.Price == nil && microdataPrice != "" {
		if p, ok := parsePriceString(microdataPrice); ok {
			info.Price = &p
		}
	}

	var ldjsonImage string
	for _, block := range ldjsonBlocks {
		if v, ok := findJSONImage(block); ok {
			ldjsonImage = v
			break
		}
	}
	for _, candidate := range [...]string{ogImage, ldjsonImage, twitterImage} {
		if candidate == "" {
			continue
		}
		if resolved, ok := resolveImageURL(pageURL, candidate); ok {
			info.ImageURL = resolved
			break
		}
	}

	return info, nil
}

// metaContent reads a <meta> tag's "content" attribute when its attrKey
// (e.g. "property" or "name") equals attrVal (e.g. "og:image").
func metaContent(n *html.Node, attrKey, attrVal string) (string, bool) {
	var key, content string
	for _, a := range n.Attr {
		switch a.Key {
		case attrKey:
			key = a.Val
		case "content":
			content = a.Val
		}
	}
	if content == "" || key != attrVal {
		return "", false
	}
	return content, true
}

// resolveImageURL turns a possibly-relative image URL (as found in HTML
// markup) into an absolute http(s) URL relative to pageURL, rejecting
// anything else (a "javascript:"/"data:" URI could otherwise end up here
// the same way it's blocked from item.url by internal/validate.URL).
func resolveImageURL(pageURL *url.URL, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	resolved := pageURL.ResolveReference(ref)
	scheme := strings.ToLower(resolved.Scheme)
	if scheme != "http" && scheme != "https" || resolved.Host == "" {
		return "", false
	}
	return resolved.String(), true
}

// metaPrice recognizes the OpenGraph/Facebook product price tags
// (<meta property="og:price:amount" content="...">, and its
// "product:price:amount" alias used by some storefronts).
func metaPrice(n *html.Node) (string, bool) {
	var property, content string
	for _, a := range n.Attr {
		switch a.Key {
		case "property":
			property = a.Val
		case "content":
			content = a.Val
		}
	}
	if content == "" {
		return "", false
	}
	switch property {
	case "og:price:amount", "product:price:amount":
		return content, true
	default:
		return "", false
	}
}

func isLDJSON(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key == "type" && strings.EqualFold(a.Val, "application/ld+json") {
			return true
		}
	}
	return false
}

// hasItemPropPrice recognizes the Schema.org microdata convention
// (itemprop="price"), used either on the element carrying the value as a
// "content" attribute (e.g. <span itemprop="price" content="19.99">) or,
// failing that, the element's own text.
func hasItemPropPrice(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key == "itemprop" && a.Val == "price" {
			return true
		}
	}
	return false
}

func microdataValue(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "content" {
			return a.Val
		}
	}
	return textContent(n)
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// findJSONPrice parses a JSON-LD block (a single Schema.org object, or an
// array/@graph of them) and searches it for a "price" field, as found on a
// Product's nested Offer (https://schema.org/price).
func findJSONPrice(raw string) (float64, bool) {
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return 0, false
	}
	return searchJSONPrice(data)
}

func searchJSONPrice(v any) (float64, bool) {
	switch val := v.(type) {
	case map[string]any:
		for _, key := range [...]string{"price", "priceAmount", "lowPrice"} {
			if raw, ok := val[key]; ok {
				if p, ok := coerceJSONPrice(raw); ok {
					return p, true
				}
			}
		}
		for _, child := range val {
			if p, ok := searchJSONPrice(child); ok {
				return p, true
			}
		}
	case []any:
		for _, child := range val {
			if p, ok := searchJSONPrice(child); ok {
				return p, true
			}
		}
	}
	return 0, false
}

// findJSONImage parses a JSON-LD block and searches it for an "image"
// field, as found on a Product (https://schema.org/image). The value may
// be a plain string, an array of strings (the first is used), or a nested
// ImageObject with its own "url" field — searchJSONImage handles all three.
func findJSONImage(raw string) (string, bool) {
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", false
	}
	return searchJSONImage(data)
}

func searchJSONImage(v any) (string, bool) {
	switch val := v.(type) {
	case map[string]any:
		if raw, ok := val["image"]; ok {
			if s, ok := coerceJSONImage(raw); ok {
				return s, true
			}
		}
		for key, child := range val {
			if key == "image" {
				continue // already tried above; avoid matching an unrelated nested "image"
			}
			if s, ok := searchJSONImage(child); ok {
				return s, true
			}
		}
	case []any:
		for _, child := range val {
			if s, ok := searchJSONImage(child); ok {
				return s, true
			}
		}
	}
	return "", false
}

// coerceJSONImage normalizes the three shapes schema.org's "image"
// property may take: a plain URL string, an array (first entry wins), or
// an ImageObject ({"@type": "ImageObject", "url": "..."}).
func coerceJSONImage(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		if val != "" {
			return val, true
		}
	case []any:
		for _, item := range val {
			if s, ok := coerceJSONImage(item); ok {
				return s, true
			}
		}
	case map[string]any:
		if u, ok := val["url"]; ok {
			if s, ok := u.(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

func coerceJSONPrice(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if n >= 0 {
			return n, true
		}
	case string:
		return parsePriceString(n)
	}
	return 0, false
}

// priceCleanRe strips everything but digits, dots and commas (currency
// symbols, spaces, non-breaking spaces, "€"/"$"/"EUR", etc.) before
// attempting to parse a price out of free-form text content.
var priceCleanRe = regexp.MustCompile(`[^\d.,]`)

// parsePriceString extracts a non-negative decimal amount from a
// price-shaped string, tolerating both "1234.56"/"1,234.56" (US-style)
// and "1234,56"/"1.234,56" (European-style) formatting.
func parsePriceString(raw string) (float64, bool) {
	s := priceCleanRe.ReplaceAllString(strings.TrimSpace(raw), "")
	if s == "" {
		return 0, false
	}

	switch {
	case strings.Contains(s, ",") && strings.Contains(s, "."):
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			// e.g. "1.234,56": '.' is a thousands separator, ',' is decimal.
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			// e.g. "1,234.56": ',' is a thousands separator.
			s = strings.ReplaceAll(s, ",", "")
		}
	case strings.Contains(s, ","):
		// Only a comma: treat it as a decimal separator if it looks like one
		// (a single comma, at most two digits after it), else as a
		// thousands separator to strip.
		parts := strings.Split(s, ",")
		if len(parts) == 2 && len(parts[1]) <= 2 {
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil || val < 0 {
		return 0, false
	}
	return val, true
}
