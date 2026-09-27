package facebook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/saucesteals/shop"
)

const maxGraphQLSize = 16 << 20

func (s *Store) graphQL(ctx context.Context, docID, queryName string, variables json.RawMessage) (json.RawMessage, error) {
	form := url.Values{
		"__a":                      {"1"},
		"__comet_req":              {"15"},
		"doc_id":                   {docID},
		"fb_api_caller_class":      {"RelayModern"},
		"fb_api_req_friendly_name": {queryName},
		"server_timestamps":        {"true"},
		"variables":                {string(variables)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/graphql/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build Marketplace GraphQL request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/marketplace/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "request Marketplace GraphQL: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	location := strings.ToLower(resp.Header.Get("Location"))
	switch {
	case resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode >= 300 && resp.StatusCode < 400 && (strings.Contains(location, "/login") || strings.Contains(location, "/checkpoint")):
		return nil, shop.Errorf(shop.ErrAuthRequired, "marketplace requires authentication")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, shop.Errorf(shop.ErrRateLimited, "marketplace rate limited")
	case resp.StatusCode != http.StatusOK:
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace GraphQL returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGraphQLSize+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read Marketplace GraphQL: %v", err)
	}
	if len(body) > maxGraphQLSize {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace GraphQL response exceeds size limit")
	}
	raw, err := decodeGraphQL(body)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Error            int    `json:"error"`
		ErrorSummary     string `json:"errorSummary"`
		ErrorDescription string `json:"errorDescription"`
		Errors           []struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, shop.Errorf(shop.ErrUpstream, "decode Marketplace GraphQL envelope: %v", err)
	}
	if envelope.Error != 0 {
		message := strings.TrimSpace(envelope.ErrorDescription)
		if message == "" {
			message = strings.TrimSpace(envelope.ErrorSummary)
		}
		if message == "" {
			message = fmt.Sprintf("error %d", envelope.Error)
		}
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace GraphQL: %s", message)
	}
	if len(envelope.Errors) > 0 {
		failure := envelope.Errors[0]
		if failure.Code == 1675004 || strings.Contains(strings.ToLower(failure.Message), "rate limit") {
			return nil, shop.Errorf(shop.ErrRateLimited, "marketplace GraphQL rate limited")
		}
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace GraphQL: %s", failure.Message)
	}

	return raw, nil
}

func decodeGraphQL(body []byte) (json.RawMessage, error) {
	body = bytes.TrimSpace(body)
	body = bytes.TrimPrefix(body, []byte("for (;;);"))
	body = bytes.TrimSpace(body)
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, shop.Errorf(shop.ErrUpstream, "decode Marketplace GraphQL response: %v", err)
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, shop.Errorf(shop.ErrUpstream, "invalid Marketplace GraphQL response")
	}

	return raw, nil
}
