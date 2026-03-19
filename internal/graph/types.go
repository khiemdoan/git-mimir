package graph

import (
	"crypto/sha256"
	"fmt"
)

// EdgeType constants.
const (
	EdgeCalls      = "CALLS"
	EdgeImports    = "IMPORTS"
	EdgeExtends    = "EXTENDS"
	EdgeImplements = "IMPLEMENTS"
	EdgeMemberOf   = "MEMBER_OF"
)

// Node represents a code symbol in the knowledge graph.
type Node struct {
	UID         string
	Name        string
	Kind        string // Function|Method|Class|Interface|Variable|Constant|Type
	FilePath    string
	StartLine   uint
	EndLine     uint
	Exported    bool
	PackagePath string
	ClusterID   string
}

// Edge represents a directed relationship between two nodes.
type Edge struct {
	FromUID    string
	ToUID      string
	Type       string  // CALLS|IMPORTS|EXTENDS|IMPLEMENTS|MEMBER_OF
	Confidence float64 // 0.0–1.0
}

// NodeUID generates a stable UID for a node from its file, name, and kind.
// Uses first 16 hex chars of SHA-256.
func NodeUID(filePath, name, kind string) string {
	h := sha256.Sum256([]byte(filePath + ":" + name + ":" + kind))
	return fmt.Sprintf("%x", h[:8])
}

// EdgeID generates a stable edge ID from the three key fields.
func EdgeID(fromUID, toUID, edgeType string) string {
	h := sha256.Sum256([]byte(fromUID + "|" + toUID + "|" + edgeType))
	return fmt.Sprintf("%x", h[:8])
}

// FileSymbols holds all symbols extracted from a single file.
type FileSymbols struct {
	FilePath string
	Nodes    []Node
	Imports  []ImportRef // raw import statements for resolver pass
	Calls    []CallRef   // raw call expressions for resolver pass
}

// ImportRef is a raw import extracted by the parser (pre-resolution).
type ImportRef struct {
	FromUID    string // node UID of the containing symbol
	ImportPath string // e.g. "./greeter" or "fmt"
	Alias      string // import alias if present
	IsType     bool   // TypeScript `import type`
}

// CallRef is a raw call expression extracted by the parser (pre-resolution).
type CallRef struct {
	FromUID    string // node UID of the calling symbol
	CalleeName string // name of the callee as written in source
	Line       uint
}
