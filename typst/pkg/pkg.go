// pkg handles typst package/template, either remote or local.
package pkg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	tpix "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/storage"
	"looz.ws/typstify/service/settings"
)

type TypstPkg struct {
	api.SearchResult
	// If the package is a remote package, it may have beed cached.
	IsCached bool
	Versions []api.PackageVersionInfo
}

func (p *TypstPkg) ImportPath() string {
	return fmt.Sprintf("@%s/%s:%s", p.Namespace, p.Name, p.LatestVersion)
}

type TypstPkgService struct {
	//	cacheDir    string
	pkgCache    storage.Store
	tpixConfig  *settings.TpixSettings
	tpixClient  *tpix.TpixSdk
	searchState searchState
}

// searchState caches the latest TPIX search response. Searches are issued from
// background goroutines, so the cache is guarded by a mutex.
type searchState struct {
	mu   sync.Mutex
	key  string
	data *api.SearchResponse
}

func (p *TypstPkg) ThumbUrl(size string) string {
	if !p.IsTemplate {
		return ""
	}

	if size == "" {
		return fmt.Sprintf("https://packages.typst.org/preview/thumbnails/%s-%s.webp", p.Name, p.Versions[0].Version)
	}
	return fmt.Sprintf("https://packages.typst.org/preview/thumbnails/%s-%s-%s.webp", p.Name, p.Versions[0].Version, size)
}

func ImportPath(namespace string, name string, version string) string {
	return fmt.Sprintf("@%s/%s:%s", namespace, name, version)
}

func DefaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "typst", "packages")
}

func NewTypstPkgService(config *settings.TypstSettings, tpixConfig *settings.TpixSettings, reporterWriter io.Writer) *TypstPkgService {
	cacheDir := config.PackageCacheDir

	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}

	store, err := storage.NewFsPackageStore(cacheDir)
	if err != nil {
		panic("Cannot build Typst package cache: " + cacheDir)
	}

	httpClient := api.NewHttpClient(&tpixApiKeyProvider{setting: tpixConfig})
	client := tpix.NewTpixSdk(httpClient, store)
	client.WithReporter(tpixCliReporter{w: reporterWriter}.Report)

	return &TypstPkgService{
		pkgCache:   store,
		tpixConfig: tpixConfig,
		tpixClient: client,
	}
}

// Create a empty package using builtin template manifest. Returning the dir of
// of package, and a optional error.
func (s *TypstPkgService) CreatePkg(pkgDir string, name string, isTemplate bool) (string, error) {
	author := fmt.Sprintf("%s <%s>", s.tpixConfig.Username, s.tpixConfig.Email)
	return CreatePkg(pkgDir, name, isTemplate, author)
}

func (s *TypstPkgService) CreateSampleDocument(projectDir string, name string) (string, error) {
	author := fmt.Sprintf("%s <%s>", s.tpixConfig.Username, s.tpixConfig.Email)
	return createTemplateDocument(projectDir, name, author)
}

func (s *TypstPkgService) CachedPkgs() ([]TypstPkg, error) {
	pkgMap, err := scanPackages(s.pkgCache)
	if err != nil {
		return nil, err
	}

	list := make([]TypstPkg, 0)
	for _, p := range pkgMap {
		list = append(list, p)
	}
	return list, nil
}

func (s *TypstPkgService) Download(namespace string, name string, version string) (string, int, error) {
	spec := fmt.Sprintf("@%s/%s", namespace, name)
	if version != "" {
		spec += ":" + version
	}

	deps, err := s.tpixClient.DownloadPackage(spec, false)
	if err != nil {
		return "", 0, err
	}

	path, err := s.pkgCache.Path(deps[0].Package)
	if err != nil {
		return "", 0, err
	}
	return path, len(deps), nil
}

func (s *TypstPkgService) DownloadWithSpec(spec string) (string, int, error) {
	deps, err := s.tpixClient.DownloadPackage(spec, false)
	if err != nil {
		return "", 0, err
	}

	path, err := s.pkgCache.Path(deps[0].Package)
	if err != nil {
		return "", 0, err
	}
	return path, len(deps), nil
}

func (s *TypstPkgService) PullDependencies(projectDir string) error {
	_, err := s.tpixClient.DownloadProjectDependencies(projectDir, false)
	return err
}

func (s *TypstPkgService) Bundle(projectDir string, outputDir string) (string, error) {
	outputFile := filepath.Join(outputDir, filepath.Base(projectDir)+".tar.gz")

	return s.tpixClient.BundlePackage(projectDir, outputFile, nil)
}

func (s *TypstPkgService) Push(packagePath string, namespace string) error {
	_, err := s.tpixClient.PushPackage(packagePath, namespace)
	return err
}

func (s *TypstPkgService) AccessibleNamesapces() ([]api.UserNamespace, error) {
	profile, err := s.tpixClient.GetUserProfile()
	if err != nil {
		return nil, err
	}

	return profile.Namespaces, nil
}

func (s *TypstPkgService) GetPkgDetail(pkgSpec string) (TypstPkg, error) {
	resp, err := s.tpixClient.QueryPackage(pkgSpec)
	if err != nil {
		return TypstPkg{}, err
	}

	latestVersion := ""
	if len(resp.Versions) > 0 {
		latestVersion = resp.Versions[0].Version
	}

	exists, err := s.pkgCache.Has(deps.Dependency{Namespace: resp.Namespace, Name: resp.Name, Version: latestVersion})

	return TypstPkg{
		SearchResult: api.SearchResult{
			Name:          resp.Name,
			Namespace:     resp.Namespace,
			Description:   resp.Description,
			LatestVersion: latestVersion,
			PublishedAt:   resp.LastPublishedAt,
			License:       resp.License,
			IsTemplate:    resp.IsTemplate,
			CreatedAt:     resp.CreatedAt,
		},
		IsCached: err == nil && exists,
		Versions: resp.Versions,
	}, nil
}

func (s *TypstPkgService) PkgIndexForLLM() (string, error) {
	return s.tpixClient.GetPackageIndex()
}

func (s *TypstPkgService) TpixClient() *tpix.TpixSdk {
	return s.tpixClient
}
