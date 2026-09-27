package facebook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/saucesteals/shop"
)

const maxJavaScriptSize = 8 << 20

var operationExport = regexp.MustCompile(`exports="([0-9]+)"`)

type bootloaderResource struct {
	Type string `json:"type"`
	Src  string `json:"src"`
}

func (s *Store) operationID(ctx context.Context, body []byte, component, module string) (string, error) {
	resources, err := operationResources(body, component)
	if err != nil {
		return "", err
	}
	resourceKey := strings.Join(resources, "\x00")
	s.cacheMu.RLock()
	cached := s.operations[module]
	s.cacheMu.RUnlock()
	if cached.resources == resourceKey && cached.id != "" {
		return cached.id, nil
	}

	id, err := s.discoverOperationID(ctx, resources, module)
	if err != nil {
		return "", err
	}
	s.cacheMu.Lock()
	s.operations[module] = cachedOperation{
		resources: resourceKey,
		id:        id,
	}
	s.cacheMu.Unlock()

	return id, nil
}

func (s *Store) discoverOperationID(ctx context.Context, resources []string, module string) (string, error) {
	var lastErr error
	for _, sourceURL := range resources {
		script, err := s.javaScript(ctx, sourceURL)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = err
			continue
		}
		if id := operationIDFromScript(script, module); id != "" {
			return id, nil
		}
	}
	if lastErr != nil {
		return "", lastErr
	}

	return "", shop.Errorf(shop.ErrUpstream, "marketplace client operation %s missing", module)
}

func operationResources(body []byte, component string) ([]string, error) {
	resources, err := resourcesForComponent(body, component)
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(resources))
	for _, resource := range resources {
		sourceURL, ok := resourceURL(body, resource)
		if !ok {
			continue
		}
		urls = append(urls, sourceURL)
	}
	if len(urls) == 0 {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace client resource URLs for %s missing", component)
	}

	return urls, nil
}

func resourcesForComponent(body []byte, component string) ([]string, error) {
	raw, ok := jsonObjectForKey(body, component)
	if !ok {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace client component %s missing", component)
	}
	resources := make([]string, 0)
	seen := make(map[string]bool)
	if err := collectResourceLists(raw, &resources, seen); err != nil {
		return nil, shop.Errorf(shop.ErrUpstream, "decode Marketplace client resources: %v", err)
	}
	if len(resources) == 0 {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace client resources for %s missing", component)
	}

	return resources, nil
}

func collectResourceLists(raw json.RawMessage, resources *[]string, seen map[string]bool) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	for _, key := range sortedKeys(object) {
		value := object[key]
		if key == "r" {
			var names []string
			if err := json.Unmarshal(value, &names); err != nil {
				return err
			}
			for _, name := range names {
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				*resources = append(*resources, name)
			}
			continue
		}
		value = bytes.TrimSpace(value)
		if len(value) > 0 && value[0] == '{' {
			if err := collectResourceLists(value, resources, seen); err != nil {
				return err
			}
		}
	}

	return nil
}

func resourceURL(body []byte, resource string) (string, bool) {
	if sourceURL := scriptURLForHash(body, resource); sourceURL != "" {
		return sourceURL, true
	}
	raw, ok := jsonObjectForKey(body, resource)
	if !ok {
		return "", false
	}
	var value bootloaderResource
	if err := json.Unmarshal(raw, &value); err != nil || value.Type != "js" || value.Src == "" {
		return "", false
	}

	return value.Src, true
}

func scriptURLForHash(body []byte, resource string) string {
	tokens := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken:
			token := tokens.Token()
			if token.Data != "script" {
				continue
			}
			hash := ""
			sourceURL := ""
			for _, attribute := range token.Attr {
				switch attribute.Key {
				case "data-bootloader-hash":
					hash = attribute.Val
				case "src":
					sourceURL = attribute.Val
				}
			}
			if hash == resource {
				return sourceURL
			}
		}
	}
}

func jsonObjectForKey(body []byte, key string) (json.RawMessage, bool) {
	needle := []byte(strconv.Quote(key) + ":")
	for offset := 0; offset < len(body); {
		index := bytes.Index(body[offset:], needle)
		if index < 0 {
			return nil, false
		}
		start := offset + index + len(needle)
		for start < len(body) && (body[start] == ' ' || body[start] == '\n' || body[start] == '\r' || body[start] == '\t') {
			start++
		}
		if start < len(body) && body[start] == '{' {
			if raw, ok := jsonObjectAt(body, start); ok {
				return raw, true
			}
		}
		offset = start
	}

	return nil, false
}

func jsonObjectAt(body []byte, start int) (json.RawMessage, bool) {
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(body); index++ {
		character := body[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case character == '\\':
				escaped = true
			case character == '"':
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				raw := body[start : index+1]
				return raw, json.Valid(raw)
			}
		}
	}

	return nil, false
}

func (s *Store) javaScript(ctx context.Context, value string) ([]byte, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || !strings.HasSuffix(parsed.Hostname(), ".fbcdn.net") {
		return nil, shop.Errorf(shop.ErrUpstream, "invalid Marketplace client resource URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Marketplace client resource request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "request Marketplace client resource: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace client resource returned HTTP %d", resp.StatusCode)
	}
	script, err := io.ReadAll(io.LimitReader(resp.Body, maxJavaScriptSize+1))
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read Marketplace client resource: %v", err)
	}
	if len(script) > maxJavaScriptSize {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace client resource exceeds size limit")
	}

	return script, nil
}

func operationIDFromScript(script []byte, module string) string {
	start := bytes.Index(script, []byte(`__d(`+strconv.Quote(module)))
	if start < 0 {
		return ""
	}
	end := min(start+1024, len(script))
	match := operationExport.FindSubmatch(script[start:end])
	if len(match) != 2 {
		return ""
	}

	return string(match[1])
}
