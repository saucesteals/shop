package facebook

import (
	"context"
	"time"
)

type cachedSearchPage struct {
	page      searchPage
	resources []string
	expires   time.Time
}

// searchPage caches only bootstrap metadata, never listing results. Resource
// URLs are retained for lazy pagination discovery, not the HTML document.
func (s *Store) searchPage(ctx context.Context, path string) (searchPage, []string, error) {
	s.cacheMu.RLock()
	cached, ok := s.searchPages[path]
	s.cacheMu.RUnlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.page, cached.resources, nil
	}
	body, err := s.document(ctx, path)
	if err != nil {
		return searchPage{}, nil, err
	}
	page, err := parseSearchPage(body)
	if err != nil {
		return searchPage{}, nil, err
	}
	// First-page search does not require pagination resources.
	resources, _ := operationResources(body, searchContentComponent)
	s.cacheMu.Lock()
	if s.searchPages == nil || len(s.searchPages) >= 64 {
		s.searchPages = make(map[string]cachedSearchPage)
	}
	s.searchPages[path] = cachedSearchPage{page: page, resources: resources, expires: time.Now().Add(time.Hour)}
	s.cacheMu.Unlock()
	return page, resources, nil
}

func (s *Store) invalidateSearchPage(path string) {
	s.cacheMu.Lock()
	delete(s.searchPages, path)
	s.cacheMu.Unlock()
}
