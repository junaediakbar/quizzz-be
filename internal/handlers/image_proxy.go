package handlers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

const defaultImageProxyMaxMB = 12

var defaultImageProxyHosts = []string{
	"i.ibb.co",
	"ibb.co",
	"image.ibb.co",
	"res.cloudinary.com",
}

var (
	imageProxyClient     *http.Client
	imageProxyClientOnce sync.Once
)

// ProxyImage fetches an external HTTPS image and streams it through the API (SSRF-safe allowlist).
func (h *MediaHandler) ProxyImage(c *fiber.Ctx) error {
	rawURL := strings.TrimSpace(c.Query("url"))
	if rawURL == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "query parameter 'url' is required"})
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "url must be a valid https URL"})
	}

	host := strings.ToLower(parsed.Hostname())
	if !isAllowedImageProxyHost(host) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "image host is not allowed"})
	}

	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "image host is not allowed"})
	}

	maxBytes := int64(defaultImageProxyMaxMB) * 1024 * 1024
	if v := os.Getenv("IMAGE_PROXY_MAX_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxBytes = int64(n) * 1024 * 1024
		}
	}

	client := imageProxyHTTPClient()

	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid url"})
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; QuizApp-ImageProxy/1.0)")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://imgbb.com/")

	resp, err := client.Do(req)
	if err != nil {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": "failed to fetch image"})
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{
			"error": fmt.Sprintf("upstream returned %d", resp.StatusCode),
		})
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		// ImgBB kadang mengembalikan application/octet-stream untuk file .png
		lowerPath := strings.ToLower(parsed.Path)
		if !(strings.HasSuffix(lowerPath, ".png") ||
			strings.HasSuffix(lowerPath, ".jpg") ||
			strings.HasSuffix(lowerPath, ".jpeg") ||
			strings.HasSuffix(lowerPath, ".webp") ||
			strings.HasSuffix(lowerPath, ".gif")) {
			return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": "upstream response is not an image"})
		}
		if contentType == "" {
			switch {
			case strings.HasSuffix(lowerPath, ".png"):
				contentType = "image/png"
			case strings.HasSuffix(lowerPath, ".gif"):
				contentType = "image/gif"
			case strings.HasSuffix(lowerPath, ".webp"):
				contentType = "image/webp"
			default:
				contentType = "image/jpeg"
			}
		}
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": "failed to read image"})
	}
	if int64(len(body)) > maxBytes {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": "image too large"})
	}

	c.Set("Content-Type", contentType)
	c.Set("Cache-Control", "public, max-age=86400, immutable")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Send(body)
}

// imageProxyHTTPClient uses public DNS (1.1.1.1) to avoid ISP DNS hijacking (e.g. Internet Positif).
func imageProxyHTTPClient() *http.Client {
	imageProxyClientOnce.Do(func() {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				// Coba Cloudflare DNS, fallback ke Google DNS
				conn, err := d.DialContext(ctx, "udp", "1.1.1.1:53")
				if err != nil {
					return d.DialContext(ctx, "udp", "8.8.8.8:53")
				}
				return conn, nil
			},
		}
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Resolver:  resolver,
		}
		transport := &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			MaxIdleConns:          16,
		}
		imageProxyClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				nextHost := strings.ToLower(req.URL.Hostname())
				if !isAllowedImageProxyHost(nextHost) {
					return fmt.Errorf("redirect host not allowed")
				}
				return nil
			},
		}
	})
	return imageProxyClient
}

func isAllowedImageProxyHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, allowed := range imageProxyAllowedHosts() {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func imageProxyAllowedHosts() []string {
	if v := strings.TrimSpace(os.Getenv("IMAGE_PROXY_ALLOWED_HOSTS")); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return defaultImageProxyHosts
}
