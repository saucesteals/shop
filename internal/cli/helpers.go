package cli

import "strings"

// parseKeyValues converts ["key1=val1", "key2=val2"] into a map.
func parseKeyValues(pairs []string) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		m[k] = v
	}

	return m
}
