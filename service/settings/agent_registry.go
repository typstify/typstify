package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

const registryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

type AgentRegistry struct {
	Version string       `json:"version"`
	Agents  []AgentEntry `json:"agents"`
}

type AgentEntry struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Description  string             `json:"description"`
	Repository   string             `json:"repository,omitempty"`
	Website      string             `json:"website,omitempty"`
	Authors      []string           `json:"authors"`
	License      string             `json:"license"`
	Icon         string             `json:"icon"`
	Distribution AgentDistribution `json:"distribution"`
}

// DistKind returns "npx", "uvx", "binary", or "unknown".
func (e AgentEntry) DistKind() string {
	if e.Distribution.Npx != nil {
		return "npx"
	}
	if e.Distribution.Uvx != nil {
		return "uvx"
	}
	if len(e.Distribution.Binary) > 0 {
		return "binary"
	}
	return "unknown"
}

// AgentDistribution is a one-of: exactly one of Npx, Uvx, or Binary is set.
type AgentDistribution struct {
	Npx    *PackageDistro            `json:"npx,omitempty"`
	Uvx    *PackageDistro            `json:"uvx,omitempty"`
	Binary map[string]*BinaryDistro  `json:"binary,omitempty"`
}

// PackageDistro holds the fields for npx and uvx distributions.
type PackageDistro struct {
	Package string            `json:"package"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BinaryDistro holds a single platform binary distribution.
type BinaryDistro struct {
	Archive string            `json:"archive"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

var (
	cachedRegistry *AgentRegistry
	registryMu     sync.Mutex
)

// FetchAgentRegistry fetches the ACP agent registry from the CDN.
// Results are cached in memory after the first successful fetch.
func FetchAgentRegistry(ctx context.Context) (*AgentRegistry, error) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if cachedRegistry != nil {
		return cachedRegistry, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var reg AgentRegistry
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, err
	}

	cachedRegistry = &reg
	return cachedRegistry, nil
}

// LookupAgent finds a registry entry by ID. Returns nil if not found.
func LookupAgent(id string) *AgentEntry {
	registryMu.Lock()
	defer registryMu.Unlock()

	if cachedRegistry == nil {
		return nil
	}
	for i := range cachedRegistry.Agents {
		if cachedRegistry.Agents[i].ID == id {
			return &cachedRegistry.Agents[i]
		}
	}
	return nil
}
