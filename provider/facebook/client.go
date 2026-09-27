package facebook

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/saucesteals/shop"
)

const maxDocumentSize = 16 << 20

func (s *Store) document(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build marketplace request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-CH-UA", `"Not(A:Brand";v="99", "Google Chrome";v="133", "Chromium";v="133"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "request marketplace document: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	location := strings.ToLower(resp.Header.Get("Location"))
	switch {
	case resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode >= 300 && resp.StatusCode < 400 && (strings.Contains(location, "/login") || strings.Contains(location, "/checkpoint")):
		return nil, shop.Errorf(shop.ErrAuthRequired, "marketplace requires authentication")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, shop.Errorf(shop.ErrRateLimited, "marketplace rate limited")
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil, shop.Errorf(shop.ErrNotFound, "unknown Marketplace city")
	case resp.StatusCode != http.StatusOK:
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentSize+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read marketplace document: %v", err)
	}
	if len(body) > maxDocumentSize {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace document exceeds size limit")
	}

	return body, nil
}
