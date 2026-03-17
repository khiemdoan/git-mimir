package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/mimir/internal/registry"
)

// JSON-RPC 2.0 message types
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

// Server is the MCP JSON-RPC stdio server.
type Server struct {
	reg     *registry.Registry
	tools   *Tools
	encoder *json.Encoder
}

// Serve starts the MCP stdio server and blocks until SIGTERM.
func Serve(reg *registry.Registry) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	srv := &Server{
		reg:     reg,
		tools:   NewTools(reg),
		encoder: json.NewEncoder(os.Stdout),
	}

	return srv.run(ctx, os.Stdin)
}

func (s *Server) run(ctx context.Context, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil // EOF
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, ErrParseError, "Parse error: "+err.Error())
			continue
		}

		// Notification (no id) — handle but don't respond
		if req.ID == nil {
			s.handleNotification(ctx, &req)
			continue
		}

		resp := s.handleRequest(ctx, &req)
		resp.JSONRPC = "2.0"
		resp.ID = req.ID
		if err := s.encoder.Encode(resp); err != nil {
			log.Printf("mcp: encode response: %v", err)
		}
	}
}

func (s *Server) handleNotification(ctx context.Context, req *Request) {
	switch req.Method {
	case "notifications/initialized":
		// nothing
	case "notifications/cancelled":
		// nothing
	}
}

func (s *Server) handleRequest(ctx context.Context, req *Request) Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return Response{Result: s.tools.ListTools()}
	case "tools/call":
		return s.tools.Call(ctx, req.Params)
	case "resources/list":
		return Response{Result: listResources(s.reg)}
	case "resources/read":
		return readResource(ctx, req.Params, s.reg)
	case "prompts/list":
		return Response{Result: listPrompts()}
	case "prompts/get":
		return getPrompt(req.Params)
	default:
		return Response{Error: &RPCError{Code: ErrMethodNotFound, Message: fmt.Sprintf("method not found: %s", req.Method)}}
	}
}

func (s *Server) handleInitialize(req *Request) Response {
	return Response{
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{"listChanged": false},
				"resources": map[string]interface{}{"listChanged": false},
				"prompts":   map[string]interface{}{"listChanged": false},
			},
			"serverInfo": map[string]interface{}{
				"name":    "mimir",
				"version": "1.0.0",
			},
		},
	}
}

func (s *Server) sendError(id *json.RawMessage, code int, msg string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
	s.encoder.Encode(resp)
}
