package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/yourusername/mimir/internal/registry"
	"github.com/yourusername/mimir/internal/store"
)

const defaultPort = 7842

// Server is the HTTP bridge server.
type Server struct {
	reg  *registry.Registry
	mux  *http.ServeMux
	port int
}

// NewServer creates a new HTTP server.
func NewServer(reg *registry.Registry, port int) *Server {
	if port == 0 {
		port = defaultPort
	}
	s := &Server{reg: reg, mux: http.NewServeMux(), port: port}
	s.routes()
	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("Mimir HTTP bridge listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, corsMiddleware(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", serveUI)
	s.mux.HandleFunc("/repos", s.handleRepos)
	s.mux.HandleFunc("/repo/", s.handleRepo)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]interface{}{"repos": s.reg.List()})
}

func (s *Server) handleRepo(w http.ResponseWriter, r *http.Request) {
	// Path: /repo/{name}/{action}[/{param}]
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/repo/"), "/", 3)
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	repoName := parts[0]
	action := parts[1]
	param := ""
	if len(parts) > 2 {
		param = parts[2]
	}

	st, err := s.openStore(repoName)
	if err != nil {
		http.Error(w, "store error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer st.Close()

	switch action {
	case "query":
		s.handleQuery(w, r, st)
	case "context":
		s.handleContext(w, r, st, param)
	case "impact":
		s.handleImpact(w, r, st, param)
	case "clusters":
		s.handleClusters(w, r, st)
	case "cypher":
		s.handleCypher(w, r, st)
	case "graph":
		s.handleGraph(w, r, st)
	case "processes":
		s.handleProcesses(w, r, st)
	default:
		http.Error(w, "unknown action: "+action, http.StatusNotFound)
	}
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request, st *store.Store) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, map[string]interface{}{"results": []interface{}{}})
		return
	}
	terms := strings.Fields(strings.ToLower(q))
	results, err := st.HybridSearch(terms, nil, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"results": results})
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request, st *store.Store, symbol string) {
	if symbol == "" {
		http.Error(w, "symbol required", http.StatusBadRequest)
		return
	}
	nodes, err := st.QuerySymbol(symbol)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(nodes) == 0 {
		writeJSON(w, map[string]interface{}{"symbol": nil})
		return
	}
	node := nodes[0]
	outEdges, _ := st.QueryEdgesFrom(node.UID)
	inEdges, _ := st.QueryEdgesTo(node.UID)
	writeJSON(w, map[string]interface{}{
		"symbol":   node,
		"outgoing": outEdges,
		"incoming": inEdges,
	})
}

func (s *Server) handleImpact(w http.ResponseWriter, r *http.Request, st *store.Store, target string) {
	if target == "" {
		http.Error(w, "target required", http.StatusBadRequest)
		return
	}
	nodes, err := st.QuerySymbol(target)
	if err != nil || len(nodes) == 0 {
		writeJSON(w, map[string]interface{}{"target": nil})
		return
	}
	rows, err := st.QueryImpact(nodes[0].UID, "downstream", 0.5, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"target": nodes[0], "affected": rows})
}

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request, st *store.Store) {
	clusters, err := st.AllClusters()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"clusters": clusters})
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request, st *store.Store) {
	processes, err := st.AllProcesses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"processes": processes})
}

func (s *Server) handleCypher(w http.ResponseWriter, r *http.Request, st *store.Store) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	// Delegate to cypher translator (re-use MCP logic)
	var rows [][]interface{}
	var columns []string
	err := st.ReadRaw(body.Query, func(cols []string, rs [][]interface{}) {
		columns = cols
		rows = rs
	})
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"columns": columns, "rows": rows})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request, st *store.Store) {
	nodes, err := st.AllNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	edges, err := st.AllEdges()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"nodes": nodes, "edges": edges})
}

func (s *Server) openStore(repoName string) (*store.Store, error) {
	dbPath, err := registry.DBPath(repoName)
	if err != nil {
		return nil, err
	}
	return store.OpenStore(dbPath)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
