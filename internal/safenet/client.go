package safenet

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Phase 0 Task 12: SSRF-safe HTTP client
// Validates URLs before making requests, blocking access to internal networks.

const (
	// MaxResponseSize is the maximum response body size (10MB)
	MaxResponseSize = 10 * 1024 * 1024
	// MaxRedirects is the maximum number of redirects to follow
	MaxRedirects = 3
)

// blockedCIDRs contains private/reserved IP ranges that should never be accessed
var blockedCIDRs []*net.IPNet

func init() {
	blockedRanges := []string{
		"127.0.0.0/8",     // Loopback
		"10.0.0.0/8",      // Private Class A
		"172.16.0.0/12",   // Private Class B
		"192.168.0.0/16",  // Private Class C
		"169.254.0.0/16",  // Link-local
		"100.64.0.0/10",   // Shared address space (CGNAT)
		"0.0.0.0/8",       // Current network
		"224.0.0.0/4",     // Multicast
		"240.0.0.0/4",     // Reserved
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 unique local
		"fe80::/10",       // IPv6 link-local
		"ff00::/8",        // IPv6 multicast
	}
	for _, cidr := range blockedRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			blockedCIDRs = append(blockedCIDRs, network)
		}
	}
}

// isBlockedIP checks if an IP address is in a blocked range
func isBlockedIP(ip net.IP) bool {
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// safeDialContext creates a net.Dialer that validates resolved IPs before connecting
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Resolve the hostname
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed: %w", err)
	}

	// Check all resolved IPs against blocklist
	for _, ip := range ips {
		if isBlockedIP(ip.IP) {
			return nil, fmt.Errorf("blocked: address %s resolves to private/reserved IP %s", host, ip.IP)
		}
	}

	// Connect using the first valid IP
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// NewSafeClient creates an HTTP client that prevents SSRF attacks.
func NewSafeClient() *http.Client {
	transport := &http.Transport{
		DialContext:           safeDialContext,
		TLSHandshakeTimeout:  10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConnsPerHost:   2,
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("too many redirects (max %d)", MaxRedirects)
			}
			// Validate redirect destination
			host := req.URL.Hostname()
			if host == "" {
				return fmt.Errorf("redirect to empty host")
			}
			// Block non-HTTP(S) redirects
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("blocked: redirect to non-HTTP scheme %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

// SafeGet performs a GET request with SSRF protection and response size limits.
func SafeGet(ctx context.Context, url string) ([]byte, error) {
	if err := validateURL(url); err != nil {
		return nil, err
	}

	client := NewSafeClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "WacallerAPI/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Limit response size
	limited := io.LimitReader(resp.Body, MaxResponseSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if len(data) > MaxResponseSize {
		return nil, fmt.Errorf("response too large (max %d bytes)", MaxResponseSize)
	}

	return data, nil
}

// validateURL checks that a URL is safe to fetch
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("empty URL")
	}

	// Must be HTTP or HTTPS
	lower := strings.ToLower(rawURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("blocked: only http:// and https:// URLs are allowed")
	}

	return nil
}
