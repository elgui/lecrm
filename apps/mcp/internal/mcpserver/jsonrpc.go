// Package mcpserver implements the Model Context Protocol surface for
// leCRM over a Streamable HTTP transport (a single JSON-RPC 2.0
// endpoint, runs as a Compose service rather than stdio).
//
// The wire protocol is JSON-RPC 2.0 as specified by MCP. This package
// implements the handshake (initialize / notifications/initialized),
// tools/list, tools/call, and ping. The richer mark3labs/mcp-go SDK is
// the intended dependency for the production server; the v0 skeleton
// speaks the protocol directly so the binary stays hermetic and pinned
// to the repo's Go toolchain (no SDK-driven version bump).
package mcpserver

import "encoding/json"

// protocolVersion is the MCP revision this server implements.
const protocolVersion = "2025-06-18"

// jsonRPCVersion is the only supported JSON-RPC envelope version.
const jsonRPCVersion = "2.0"

// JSON-RPC 2.0 standard error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// request is an inbound JSON-RPC message. A nil ID marks a notification
// (no response is emitted).
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *request) isNotification() bool { return len(r.ID) == 0 }

// response is an outbound JSON-RPC message. Exactly one of Result/Error
// is populated.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: jsonRPCVersion, ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, msg string) response {
	return response{JSONRPC: jsonRPCVersion, ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// initializeResult is the MCP handshake reply advertising tool support.
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// toolDef is one entry in a tools/list reply.
type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type listToolsResult struct {
	Tools []toolDef `json:"tools"`
}

// callToolParams is the params object for tools/call.
type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// callToolResult is the tools/call reply. content is a list of typed
// content blocks; the skeleton always returns a single text block whose
// body is the JSON-encoded tool output.
type callToolResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textContent(s string) []contentBlock {
	return []contentBlock{{Type: "text", Text: s}}
}
