package facebook

import (
	"bytes"
	"encoding/json"
	"io"

	"golang.org/x/net/html"

	"github.com/saucesteals/shop"
)

func relayData(body []byte, visit func(json.RawMessage) error) error {
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
			if err := walkData(raw, visit); err != nil {
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
		for _, value := range object {
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
