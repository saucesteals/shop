package facebook

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"

	"golang.org/x/net/html"

	"github.com/saucesteals/shop"
)

func relayData(body []byte, visit func(json.RawMessage) error) error {
	return jsonScripts(body, func(raw json.RawMessage) error {
		return walkData(raw, visit)
	})
}

func jsonObjects(body []byte, visit func(json.RawMessage) error) error {
	return jsonScripts(body, func(raw json.RawMessage) error {
		return walkObjects(raw, visit)
	})
}

func jsonScripts(body []byte, visit func(json.RawMessage) error) error {
	tokens := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			if tokens.Err() == io.EOF {
				return nil
			}

			return shop.Errorf(shop.ErrUpstream, "parse marketplace document: %v", tokens.Err())
		case html.StartTagToken:
			token := tokens.Token()
			if token.Data != "script" {
				continue
			}
			isJSON := false
			for _, attr := range token.Attr {
				if attr.Key == "type" && attr.Val == "application/json" {
					isJSON = true
				}
			}
			if !isJSON || tokens.Next() != html.TextToken {
				continue
			}
			raw := tokens.Text()
			if !json.Valid(raw) {
				return shop.Errorf(shop.ErrUpstream, "invalid marketplace JSON script")
			}
			if err := visit(raw); err != nil {
				return err
			}
		}
	}
}

func walkData(raw json.RawMessage, visit func(json.RawMessage) error) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	switch raw[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return shop.Errorf(shop.ErrUpstream, "parse marketplace object: %v", err)
		}
		if data, ok := object["data"]; ok {
			if err := visit(data); err != nil {
				return err
			}
		}
		for _, key := range sortedKeys(object) {
			value := object[key]
			if err := walkData(value, visit); err != nil {
				return err
			}
		}
	case '[':
		var array []json.RawMessage
		if err := json.Unmarshal(raw, &array); err != nil {
			return shop.Errorf(shop.ErrUpstream, "parse marketplace array: %v", err)
		}
		for _, value := range array {
			if err := walkData(value, visit); err != nil {
				return err
			}
		}
	}

	return nil
}

func walkObjects(raw json.RawMessage, visit func(json.RawMessage) error) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	switch raw[0] {
	case '{':
		if err := visit(raw); err != nil {
			return err
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return shop.Errorf(shop.ErrUpstream, "parse marketplace object: %v", err)
		}
		for _, key := range sortedKeys(object) {
			if err := walkObjects(object[key], visit); err != nil {
				return err
			}
		}
	case '[':
		var array []json.RawMessage
		if err := json.Unmarshal(raw, &array); err != nil {
			return shop.Errorf(shop.ErrUpstream, "parse marketplace array: %v", err)
		}
		for _, value := range array {
			if err := walkObjects(value, visit); err != nil {
				return err
			}
		}
	}

	return nil
}

func sortedKeys(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
