// pkg handles typst package/template, either remote or local.
package pkg

import (
	"fmt"
	"os"
	"path/filepath"

	cli "github.com/typstify/tpix-cli"
	tpix "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
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
	cacheDir   string
	tpixConfig *settings.TpixSettings
	remoteRepo

	reporter cli.ReportFunc
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

func NewTypstPkgService(config *settings.TypstSettings, tpixConfig *settings.TpixSettings) *TypstPkgService {
	cacheDir := config.PackageCacheDir

	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}

	return &TypstPkgService{
		cacheDir:   cacheDir,
		tpixConfig: tpixConfig,
	}
}

func (s *TypstPkgService) SetReporter(reporter cli.ReportFunc) {
	s.reporter = reporter
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
	pkgMap, err := scanPackages(s.cacheDir)
	if err != nil {
		return nil, err
	}

	list := make([]TypstPkg, 0)
	for _, p := range pkgMap {
		for _, v := range p {
			list = append(list, v)
		}
	}
	return list, nil
}

func (s *TypstPkgService) GetLocalPackagePath(pkgSpec string) string {
	namespace, name, version := tpix.ParsePkgSpec(pkgSpec)
	return filepath.Join(s.cacheDir, namespace, name, version)
}

func (s *TypstPkgService) CacheDir() string {
	return s.cacheDir
}

func (s *TypstPkgService) Download(namespace string, name string, version string) (string, int, error) {
	spec := fmt.Sprintf("@%s/%s", namespace, name)
	if version != "" {
		spec += ":" + version
	}

	return tpix.DownloadPackage(spec, s.cacheDir, false, s.reporter)
}

func (s *TypstPkgService) DownloadWithSpec(spec string) (string, int, error) {

	return tpix.DownloadPackage(spec, s.cacheDir, false, s.reporter)
}

func (s *TypstPkgService) PullDependencies(projectDir string) error {
	return tpix.DownloadProjectDependencies(projectDir, s.cacheDir, false, s.reporter)
}

func (s *TypstPkgService) Bundle(projectDir string, outputDir string) (string, error) {
	outputFile := filepath.Join(outputDir, filepath.Base(projectDir)+".tar.gz")

	return tpix.BundlePackage(projectDir, outputFile, nil)
}

func (s *TypstPkgService) Push(packagePath string, namespace string) error {
	return tpix.PushPackage(packagePath, namespace, s.reporter)
}

func (s *TypstPkgService) AccessibleNamesapces() ([]api.UserNamespace, error) {
	profile, err := tpix.GetUserProfile()
	if err != nil {
		return nil, err
	}

	return profile.Namespaces, nil
}

func (s *TypstPkgService) GetPkgDetail(pkgSpec string) (TypstPkg, error) {
	resp, err := tpix.QueryPackage(pkgSpec)
	if err != nil {
		return TypstPkg{}, err
	}

	latestVersion := ""
	if len(resp.Versions) > 0 {
		latestVersion = resp.Versions[0].Version
	}

	cachePath := filepath.Join(s.cacheDir, resp.Namespace, resp.Name, latestVersion)
	_, statErr := os.Stat(cachePath)
	isCached := statErr == nil

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
		IsCached: isCached,
		Versions: resp.Versions,
	}, nil
}

func (s *TypstPkgService) PkgIndexForLLM() (string, error) {
	return tpix.GetPackageIndex()
}
