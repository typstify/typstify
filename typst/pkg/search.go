package pkg

import (
	"errors"
	"fmt"

	"github.com/typstify/tpix-cli/api"
)

// searchResultLimit caps the number of packages fetched from TPIX.
const searchResultLimit = 100

// SearchPkgs queries packages/templates from TPIX package index, this includes
// public and private namespaces the user have access permissions.
func (s *TypstPkgService) SearchPkgs(namespace string, kind string, category string, query string) ([]TypstPkg, int, error) {
	key := fmt.Sprintf("%s:%s:%s:%s", namespace, kind, category, query)

	if resp := s.cachedSearch(key); resp != nil {
		return convertToPkg(resp), resp.Total, nil
	}

	resp, err := s.tpixClient.SearchPackages(namespace, query, kind, category, "", searchResultLimit)
	if err != nil {
		return nil, 0, err
	}
	if resp == nil {
		return nil, 0, errors.New("empty search response")
	}

	s.cacheSearch(key, resp)

	return convertToPkg(resp), resp.Total, nil
}

func (s *TypstPkgService) cachedSearch(key string) *api.SearchResponse {
	s.searchState.mu.Lock()
	defer s.searchState.mu.Unlock()

	if s.searchState.key != key {
		return nil
	}

	return s.searchState.data
}

func (s *TypstPkgService) cacheSearch(key string, resp *api.SearchResponse) {
	s.searchState.mu.Lock()
	defer s.searchState.mu.Unlock()

	s.searchState.key = key
	s.searchState.data = resp
}

func convertToPkg(resp *api.SearchResponse) []TypstPkg {
	pkgs := make([]TypstPkg, len(resp.Results))
	for i, result := range resp.Results {
		pkgs[i] = TypstPkg{
			SearchResult: result,
		}
	}

	return pkgs
}
