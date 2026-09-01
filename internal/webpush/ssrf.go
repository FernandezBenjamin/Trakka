package webpush

import (
	"context"
	"fmt"
	"net"
)

// safeDialContext is a net.Dialer.DialContext replacement that resolves the
// target host itself and only ever connects to a public IP address — this
// package's SSRF defense for the same category of risk internal/scraper
// already guards against (see its own safeDialContext), just for a
// different user-influenced URL: a push subscription's Endpoint, which
// ultimately came from whatever a caller POSTed to
// POST /api/v1/push/subscribe (internal/handlers/push.go). Independently
// instantiated here rather than imported from internal/scraper, per
// CLAUDE.md's SSRF rule allowing "this guard (or an equivalent one)" — the
// two packages have no other reason to depend on each other. Resolving and
// validating here, then dialing the checked IP literal directly rather than
// letting the transport re-resolve the hostname, closes the DNS-rebinding
// TOCTOU gap a naive "resolve, check, then dial the hostname" approach would
// leave open; TLS still verifies the certificate against the original
// hostname, since net/http uses the request's own host for SNI/verification
// regardless of which IP the connection actually reached.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("splitting address %q: %w", addr, err)
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", host, err)
	}

	dialer := &net.Dialer{Timeout: requestTimeout}
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

// blockedRanges are the address ranges Go's own net.IP predicates do not
// cover but that still must never be dialed on a caller's behalf — see
// internal/scraper's identically-named constant for what each range is and
// why it matters; this list is kept in lockstep with it by hand.
var blockedRanges = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "100::/64", "2001:db8::/32",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("webpush: bad blocked CIDR " + cidr) // a constant above is malformed; a build-time bug
		}
		nets = append(nets, n)
	}
	return nets
}()

// isPublicIP reports whether ip is safe for this server to connect to on a
// caller's behalf — see internal/scraper.isPublicIP, which this mirrors
// exactly.
func isPublicIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, n := range blockedRanges {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
