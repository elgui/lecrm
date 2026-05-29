package mcpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/mcp/internal/ratelimit"
	"github.com/gbconsult/lecrm/apps/mcp/internal/store"
)

// Server is the MCP HTTP handler. It exposes the JSON-RPC endpoint on
// the configured path and a /healthz probe.
type Server struct {
	reader  store.Reader
	writer  store.Writer
	limiter *ratelimit.Limiter
	logger  *slog.Logger
	name    string
	version string
}

// Config configures a Server.
type Config struct {
	Reader store.Reader
	// Writer enables the read-write intent tools (advance_deal,
	// log_interaction, capture_lead). When nil the server is read-only: the
	// write tools are absent from the catalog and rejected if called.
	Writer  store.Writer
	Limiter *ratelimit.Limiter
	Logger  *slog.Logger
	Name    string
	Version string
}

// New builds a Server. Reader and Limiter are required.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Name == "" {
		cfg.Name = "lecrm-mcp"
	}
	if cfg.Version == "" {
		cfg.Version = "0.0.0"
	}
	return &Server{
		reader:  cfg.Reader,
		writer:  cfg.Writer,
		limiter: cfg.Limiter,
		logger:  cfg.Logger,
		name:    cfg.Name,
		version: cfg.Version,
	}
}

const maxRequestBytes = 1 << 20 // 1 MiB

// Routes returns the http.Handler mux for the MCP server.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/mcp", s.handleRPC)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleRPC implements the Streamable HTTP transport: a JSON-RPC request
// is POSTed and the response (if any) is returned as a single JSON body.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Workspace + token identity drive both authorization scope and the
	// rate-limit key. In v0 these arrive as headers set by the gateway
	// in front of the MCP service (service-token verification is shared
	// with the API per ADR-009 §4.1).
	ws, err := workspaceFromRequest(r)
	if err != nil {
		writeRPC(w, errorResponse(nil, codeInvalidRequest, err.Error()))
		return
	}
	tokenID := r.Header.Get("X-Lecrm-Token-Id")
	if !s.limiter.Allow(ws.String() + "|" + tokenID) {
		writeRPC(w, errorResponse(nil, codeInternalError, "rate limit exceeded"))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeRPC(w, errorResponse(nil, codeParseError, "request body too large or unreadable"))
		return
	}
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPC(w, errorResponse(nil, codeParseError, "invalid JSON"))
		return
	}
	if req.JSONRPC != jsonRPCVersion {
		writeRPC(w, errorResponse(req.ID, codeInvalidRequest, "jsonrpc must be 2.0"))
		return
	}

	resp, emit := s.handle(r, ws, &req)
	if !emit {
		// Notification: acknowledge with 202 and no body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, resp)
}

// handle routes one JSON-RPC request to its method handler. The second
// return value is false for notifications (no response is written).
func (s *Server) handle(r *http.Request, ws uuid.UUID, req *request) (response, bool) {
	switch req.Method {
	case "initialize":
		return resultResponse(req.ID, initializeResult{
			ProtocolVersion: protocolVersion,
			Capabilities: map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			ServerInfo: serverInfo{Name: s.name, Version: s.version},
		}), true

	case "notifications/initialized":
		return response{}, false

	case "ping":
		return resultResponse(req.ID, map[string]any{}), true

	case "tools/list":
		return resultResponse(req.ID, listToolsResult{Tools: s.toolCatalog()}), true

	case "resources/list":
		return resultResponse(req.ID, listResourcesResult{Resources: s.resourceCatalog()}), true

	case "resources/read":
		if req.isNotification() {
			return response{}, false
		}
		var p readResourceParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errorResponse(req.ID, codeInvalidParams, "invalid resource params"), true
		}
		contents, err := s.dispatchResource(r.Context(), ws, p.URI)
		if err != nil {
			// Unknown URI or a scoped read failure: an invalid-params error so
			// the client sees the message (resources have no isError channel).
			return errorResponse(req.ID, codeInvalidParams, err.Error()), true
		}
		return resultResponse(req.ID, readResourceResult{Contents: []resourceContents{contents}}), true

	case "tools/call":
		if req.isNotification() {
			return response{}, false
		}
		var p callToolParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errorResponse(req.ID, codeInvalidParams, "invalid tool params"), true
		}
		out, err := s.dispatchTool(r.Context(), ws, scopesFromRequest(r), p.Name, p.Arguments)
		if err != nil {
			// Tool-level failures are reported as isError results, not
			// transport errors, so the agent can read the message.
			return resultResponse(req.ID, callToolResult{
				Content: textContent(err.Error()),
				IsError: true,
			}), true
		}
		encoded, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return errorResponse(req.ID, codeInternalError, "encode result"), true
		}
		return resultResponse(req.ID, callToolResult{Content: textContent(string(encoded))}), true

	default:
		if req.isNotification() {
			return response{}, false
		}
		return errorResponse(req.ID, codeMethodNotFound, "unknown method: "+req.Method), true
	}
}

func writeRPC(w http.ResponseWriter, resp response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
