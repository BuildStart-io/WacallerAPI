package safenet

import (
	"context"
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blockedIPs := []string{
		"127.0.0.1",
		"127.0.0.254",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.1",
		"169.254.169.254", // Cloud metadata service
		"0.0.0.0",
		"::1",
		"fe80::1",
	}

	for _, ipStr := range blockedIPs {
		ip := net.ParseIP(ipStr)
		if !isBlockedIP(ip) {
			t.Errorf("expected IP %s to be blocked, but was allowed", ipStr)
		}
	}

	allowedIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"142.250.190.46",
	}

	for _, ipStr := range allowedIPs {
		ip := net.ParseIP(ipStr)
		if isBlockedIP(ip) {
			t.Errorf("expected IP %s to be allowed, but was blocked", ipStr)
		}
	}
}

func TestValidateURL(t *testing.T) {
	invalidURLs := []string{
		"ftp://example.com/file",
		"file:///etc/passwd",
		"gopher://example.com",
		"",
	}

	for _, url := range invalidURLs {
		if err := validateURL(url); err == nil {
			t.Errorf("expected URL %q to fail validation, but passed", url)
		}
	}

	validURLs := []string{
		"http://example.com/audio.wav",
		"https://example.com/media.png",
	}

	for _, url := range validURLs {
		if err := validateURL(url); err != nil {
			t.Errorf("expected URL %q to pass validation, but failed: %v", url, err)
		}
	}
}

func TestSSRFBlockedRequest(t *testing.T) {
	ctx := context.Background()
	_, err := SafeGet(ctx, "http://127.0.0.1:8080/secret")
	if err == nil {
		t.Fatal("expected request to 127.0.0.1 to be blocked by SSRF protection, but succeeded")
	}

	_, err = SafeGet(ctx, "http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected request to 169.254.169.254 to be blocked by SSRF protection, but succeeded")
	}
}
