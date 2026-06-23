package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	tpix "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst/pkg"
)

var _ agent.McpToolProvider = (*PackageMcpService)(nil)

type PackageMcpService struct {
	projectDir   string
	tpixSettings *settings.TpixSettings
	pkgService   *pkg.TypstPkgService

	metadataFile string
	metadataMu   sync.Mutex
}

func NewPackageMcpService(projectDir string, tpixSettings *settings.TpixSettings, pkgService *pkg.TypstPkgService) *PackageMcpService {
	return &PackageMcpService{
		projectDir:   projectDir,
		tpixSettings: tpixSettings,
		pkgService:   pkgService,
	}
}

func (ps *PackageMcpService) UserInfo(ctx context.Context) (*api.UserProfile, error) {
	if ps.tpixSettings.LoginAt <= 0 {
		return nil, errors.New("user not logged in TPIX")
	}

	return tpix.GetUserProfile()
}

// List local cached packages.
func (ps *PackageMcpService) ListLocalPackages() ([]pkg.TypstPkg, error) {
	return ps.pkgService.CachedPkgs()
}

func (ps *PackageMcpService) ensureIndexCached() (string, error) {
	ps.metadataMu.Lock()
	defer ps.metadataMu.Unlock()

	if ps.metadataFile != "" {
		if _, err := os.Stat(ps.metadataFile); err == nil {
			return ps.metadataFile, nil
		}
	}

	metadata, err := ps.pkgService.PkgIndexForLLM()
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp("", "typstify-package-index-*.md")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(metadata); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()

	ps.metadataFile = f.Name()
	return ps.metadataFile, nil
}

// ReadPackageIndex reads a line range from the cached package index file.
// offset is 0-based, limit is the number of lines to read (0 means read to end).
func (ps *PackageMcpService) ReadPackageIndex(offset, limit int) (text string, totalLines int, _ error) {
	file, err := ps.ensureIndexCached()
	if err != nil {
		return "", 0, err
	}

	f, err := os.Open(file)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	lineNum := 0
	read := 0
	for scanner.Scan() {
		if lineNum >= offset && (limit <= 0 || read < limit) {
			if read > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(scanner.Text())
			read++
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return "", 0, err
	}
	b.WriteByte('\n')
	return b.String(), lineNum, nil
}

// DownloadPackages downloads a package from TPIX server.
//
// pkgSpec should be of the pattern: `@namespace/name:version`, version can
// be omitted: `@namespace/name`. If no version is specified, the latest version
// will be downloaded.
func (ps *PackageMcpService) DownloadPackages(ctx context.Context, pkgSpec string) (pkgPath string, fileCount int, err error) {
	return ps.pkgService.DownloadWithSpec(pkgSpec)
}

func (ps *PackageMcpService) QueryPackageDetail(ctx context.Context, pkgSpec string) (pkg.TypstPkg, error) {
	return ps.pkgService.GetPkgDetail(pkgSpec)
}

func (ps *PackageMcpService) SearchPackages(ctx context.Context, queries []string, limit int) ([]pkg.TypstPkg, error) {
	if len(queries) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var results []pkg.TypstPkg

	for _, q := range queries {
		r, _, err := ps.pkgService.SearchPkgs("", "", "", q)
		if err != nil {
			continue
		}
		for _, p := range r {
			key := p.Namespace + "/" + p.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, p)
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (ps *PackageMcpService) PublishPackage(ctx context.Context, pkgDir string, targetNamespace string) error {
	tmpPkgBundleDir, err := os.MkdirTemp("", "typstify-bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpPkgBundleDir)

	outFile, err := ps.pkgService.Bundle(pkgDir, tmpPkgBundleDir)
	if err != nil {
		return err
	}

	return ps.pkgService.Push(outFile, targetNamespace)
}

func toPackageInfos(pkgs []pkg.TypstPkg) []PackageInfo {
	result := make([]PackageInfo, 0, len(pkgs))
	for _, p := range pkgs {
		versions := make([]string, len(p.Versions))
		for i, v := range p.Versions {
			versions[i] = v.Version
		}
		result = append(result, PackageInfo{
			Name:          p.Name,
			Namespace:     p.Namespace,
			Description:   p.Description,
			LatestVersion: p.LatestVersion,
			License:       p.License,
			IsTemplate:    p.IsTemplate,
			IsCached:      p.IsCached,
			ImportPath:    p.ImportPath(),
			Authors:       p.Authors,
			Categories:    p.Categories,
			Versions:      versions,
		})
	}
	return result
}

func (ps *PackageMcpService) RegisterTools(s *agent.McpServer) error {
	agent.AddMcpTool(s, listLocalPackagesTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, ListLocalPackagesResult, error) {
		result, err := ps.ListLocalPackages()
		return nil, ListLocalPackagesResult{Packages: toPackageInfos(result)}, err
	})

	agent.AddMcpTool(s, downloadPackageTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input DownloadPackageParams) (*mcpsdk.CallToolResult, DownloadPackageResult, error) {
		pkgPath, _, err := ps.DownloadPackages(ctx, input.PkgSpec)
		if err != nil {
			return nil, DownloadPackageResult{}, err
		}
		return nil, DownloadPackageResult{Path: pkgPath}, nil
	})

	agent.AddMcpTool(s, queryPackageDetailTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input QueryPackageDetailParams) (*mcpsdk.CallToolResult, any, error) {
		result, err := ps.QueryPackageDetail(ctx, input.PkgSpec)
		return nil, result, err
	})

	agent.AddMcpTool(s, searchPackagesTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input SearchPackagesParams) (*mcpsdk.CallToolResult, SearchPackagesResult, error) {
		result, err := ps.SearchPackages(ctx, input.Queries, input.Limit)
		return nil, SearchPackagesResult{Results: toPackageInfos(result)}, err
	})

	agent.AddMcpTool(s, publishPackageTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input PublishPackageParams) (*mcpsdk.CallToolResult, PublishPackageResult, error) {
		err := ps.PublishPackage(ctx, input.PkgDir, input.Namespace)
		if err != nil {
			return nil, PublishPackageResult{Success: false, Log: err.Error()}, err
		}
		return nil, PublishPackageResult{Success: true, Log: "Package published successfully"}, nil
	})

	agent.AddMcpTool(s, getUserInfoTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, any, error) {
		result, err := ps.UserInfo(ctx)
		return nil, result, err
	})

	agent.AddMcpTool(s, readPackageIndexTool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input ReadPackageMetadataParams) (*mcpsdk.CallToolResult, ReadPackageMetadataResult, error) {
		if input.Limit <= 0 {
			input.Limit = 500
		}
		text, totalLines, err := ps.ReadPackageIndex(input.Offset, input.Limit)
		if err != nil {
			return nil, ReadPackageMetadataResult{}, err
		}
		return nil, ReadPackageMetadataResult{Text: text, Offset: input.Offset, TotalLines: totalLines}, nil
	})

	return nil
}

var _ agent.McpResourceProvider = (*PackageMcpService)(nil)

func (ps *PackageMcpService) RegisterResources(s *agent.McpServer) error {
	s.AddResource(userProfileResource, func(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		profile, err := ps.UserInfo(ctx)
		if err != nil {
			return nil, mcpsdk.ResourceNotFoundError(userProfileResource.URI)
		}
		data, err := json.Marshal(profile)
		if err != nil {
			return nil, err
		}
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{{
				URI:      userProfileResource.URI,
				MIMEType: "application/json",
				Text:     string(data),
			}},
		}, nil
	})

	return nil
}
