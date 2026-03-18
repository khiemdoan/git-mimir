package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/internal/store"
)

// ResourceDefinition describes one MCP resource.
type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

func listResources(reg *registry.Registry) map[string]interface{} {
	resources := []ResourceDefinition{
		{URI: "mimir://repos", Name: "repos", Description: "List all indexed repositories", MimeType: "application/json"},
	}
	for _, repo := range reg.List() {
		name := repo.Name
		resources = append(resources,
			ResourceDefinition{
				URI:         fmt.Sprintf("mimir://repo/%s/context", name),
				Name:        name + "/context",
				Description: "Overview of " + name,
				MimeType:    "application/json",
			},
			ResourceDefinition{
				URI:         fmt.Sprintf("mimir://repo/%s/clusters", name),
				Name:        name + "/clusters",
				Description: "Community clusters for " + name,
				MimeType:    "application/json",
			},
			ResourceDefinition{
				URI:         fmt.Sprintf("mimir://repo/%s/processes", name),
				Name:        name + "/processes",
				Description: "Execution flows for " + name,
				MimeType:    "application/json",
			},
			ResourceDefinition{
				URI:         fmt.Sprintf("mimir://repo/%s/schema", name),
				Name:        name + "/schema",
				Description: "Graph schema for " + name,
				MimeType:    "application/json",
			},
		)
	}
	return map[string]interface{}{"resources": resources}
}

func readResource(ctx context.Context, params json.RawMessage, reg *registry.Registry) Response {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	uri := p.URI

	if uri == "mimir://repos" {
		return resourceResult(uri, "application/json", map[string]interface{}{"repos": reg.List()})
	}

	// Parse mimir://repo/{name}/... URIs
	repoName, subPath := parseResourceURI(uri)
	if repoName == "" {
		return errResp(ErrInvalidParams, "unknown resource: "+uri)
	}

	dbPath, err := registry.DBPath(repoName)
	if err != nil {
		return errResp(ErrInternal, err.Error())
	}
	s, err := store.OpenStore(dbPath)
	if err != nil {
		return errResp(ErrInternal, "open store: "+err.Error())
	}
	defer s.Close()

	switch subPath {
	case "context":
		nodeCount, err := s.NodeCount()
		if err != nil {
			return errResp(ErrInternal, "failed to get node count: "+err.Error())
		}
		edgeCount, err := s.EdgeCount()
		if err != nil {
			return errResp(ErrInternal, "failed to get edge count: "+err.Error())
		}
		return resourceResult(uri, "application/json", map[string]interface{}{
			"repo":       repoName,
			"node_count": nodeCount,
			"edge_count": edgeCount,
		})

	case "clusters":
		clusters, err := s.AllClusters()
		if err != nil {
			return errResp(ErrInternal, err.Error())
		}
		return resourceResult(uri, "application/json", map[string]interface{}{"clusters": clusters})

	case "processes":
		processes, err := s.AllProcesses()
		if err != nil {
			return errResp(ErrInternal, err.Error())
		}
		return resourceResult(uri, "application/json", map[string]interface{}{"processes": processes})

	case "schema":
		return resourceResult(uri, "application/json", map[string]interface{}{
			"tables":     []string{"nodes", "edges", "clusters", "cluster_members", "processes", "process_steps", "bm25_index", "embed_cache", "index_meta"},
			"node_kinds": []string{"Function", "Method", "Class", "Interface", "Variable", "Constant", "Type"},
			"edge_types": []string{"CALLS", "IMPORTS", "EXTENDS", "IMPLEMENTS", "MEMBER_OF"},
		})
	}

	// Handle mimir://repo/{name}/cluster/{id} and mimir://repo/{name}/process/{id}
	if len(subPath) > 8 && subPath[:8] == "cluster/" {
		clusterID := subPath[8:]
		clusters, err := s.AllClusters()
		if err != nil {
			return errResp(ErrInternal, err.Error())
		}
		for _, c := range clusters {
			if c.ID == clusterID {
				return resourceResult(uri, "application/json", c)
			}
		}
		return errResp(ErrInvalidParams, "cluster not found: "+clusterID)
	}

	if len(subPath) > 8 && subPath[:8] == "process/" {
		processID := subPath[8:]
		processes, err := s.AllProcesses()
		if err != nil {
			return errResp(ErrInternal, err.Error())
		}
		for _, p := range processes {
			if p.ID == processID {
				return resourceResult(uri, "application/json", p)
			}
		}
		return errResp(ErrInvalidParams, "process not found: "+processID)
	}

	return errResp(ErrInvalidParams, "unknown resource: "+uri)
}

func resourceResult(uri, mimeType string, data interface{}) Response {
	b, err := json.Marshal(data)
	if err != nil {
		return errResp(ErrInternal, "failed to marshal resource data: "+err.Error())
	}
	return Response{
		Result: map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      uri,
					"mimeType": mimeType,
					"text":     string(b),
				},
			},
		},
	}
}

// parseResourceURI parses "mimir://repo/{name}/{path}" → (name, path)
func parseResourceURI(uri string) (string, string) {
	const prefix = "mimir://repo/"
	if len(uri) <= len(prefix) {
		return "", ""
	}
	rest := uri[len(prefix):]
	idx := 0
	for idx < len(rest) && rest[idx] != '/' {
		idx++
	}
	if idx >= len(rest) {
		return rest, ""
	}
	return rest[:idx], rest[idx+1:]
}
