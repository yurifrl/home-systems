// Package mcp implements a minimal JSON-RPC 2.0 server (over stdio) that
// exposes every nostos cobra command as a tool.
//
// Single source of truth: tool definitions are derived from
// internal/cli/schema, so adding a flag in cobra automatically updates the
// MCP surface.
//
// Tool naming: MCP tool name == "nostos." + method ID (dot-separated).
//
// `tools/call` re-invokes the cobra command in-process with --output json
// and returns the captured stdout as the result content.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/schema"
)

// Request is a JSON-RPC 2.0 request envelope.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response envelope.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Tool is the tools/list element shape.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Server holds the cobra root used to invoke commands.
type Server struct {
	root *cobra.Command
}

// NewServer wires a server around the cobra root.
func NewServer(root *cobra.Command) *Server { return &Server{root: root} }

// Tools returns one Tool per registered command, sorted by name.
func (s *Server) Tools() []Tool {
	all := schema.All(s.root)
	ids := make([]string, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Tool, 0, len(ids))
	for _, id := range ids {
		m := all[id]
		props := map[string]any{}
		required := []string{}
		for _, a := range m.Args {
			props[a.Name] = map[string]any{"type": a.Type, "description": a.Description}
			if a.Required {
				required = append(required, a.Name)
			}
		}
		for _, f := range m.Flags {
			t := f.Type
			if t == "enum" {
				props[f.Name] = map[string]any{"type": "string", "enum": f.Values, "description": f.Description}
				continue
			}
			props[f.Name] = map[string]any{"type": jsonType(t), "description": f.Description}
		}
		input := map[string]any{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			input["required"] = required
		}
		out = append(out, Tool{
			Name:        "nostos." + id,
			Description: m.Description,
			InputSchema: input,
		})
	}
	return out
}

func jsonType(t string) string {
	switch t {
	case "bool":
		return "boolean"
	case "int", "int32", "int64", "duration":
		return "string"
	default:
		return "string"
	}
}

// Serve reads JSON-RPC requests from in (one per line) and writes responses to out.
// Logs go to errOut.
func (s *Server) Serve(ctx context.Context, in io.Reader, out, errOut io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.write(out, &Response{JSONRPC: "2.0", Error: &RPCError{Code: -32700, Message: "parse error: " + err.Error()}})
			continue
		}
		resp := s.dispatch(ctx, &req)
		s.write(out, resp)
	}
	return scanner.Err()
}

func (s *Server) write(out io.Writer, resp *Response) {
	if resp.JSONRPC == "" {
		resp.JSONRPC = "2.0"
	}
	b, _ := json.Marshal(resp)
	out.Write(b)
	out.Write([]byte{'\n'})
}

func (s *Server) dispatch(ctx context.Context, req *Request) *Response {
	resp := &Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "nostos", "version": s.root.Version},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.Tools()}
	case "tools/call":
		var p struct {
			Name      string                 `json:"name"`
			Arguments map[string]any         `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
			return resp
		}
		out, err := s.callTool(ctx, p.Name, p.Arguments)
		if err != nil {
			resp.Error = &RPCError{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": out},
			},
		}
	default:
		resp.Error = &RPCError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

// callTool invokes the cobra command corresponding to name.
// The command is re-rooted from a fresh tree so flags don't leak between calls.
func (s *Server) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if !strings.HasPrefix(name, "nostos.") {
		return "", fmt.Errorf("unknown tool %q (must start with nostos.)", name)
	}
	id := strings.TrimPrefix(name, "nostos.")
	parts := strings.Split(id, ".")
	cur := s.root
	path := []string{}
	for _, p := range parts {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == p {
				next = c
				break
			}
		}
		if next == nil {
			return "", fmt.Errorf("unknown command path: %s", id)
		}
		cur = next
		path = append(path, p)
	}
	// Build argv: subcommand path + --output json + flag args + positional args.
	argv := append([]string{}, path...)
	argv = append(argv, "--output", "json")
	var positional []string
	for k, v := range args {
		switch k {
		case "name", "node", "scheme", "key_id", "method", "dir":
			positional = append(positional, fmt.Sprintf("%v", v))
		default:
			switch vv := v.(type) {
			case bool:
				if vv {
					argv = append(argv, "--"+k)
				}
			case string:
				if vv != "" {
					argv = append(argv, "--"+k, vv)
				}
			default:
				argv = append(argv, "--"+k, fmt.Sprintf("%v", v))
			}
		}
	}
	argv = append(argv, positional...)

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	s.root.SetOut(buf)
	s.root.SetErr(errBuf)
	s.root.SetArgs(argv)
	defer func() {
		s.root.SetArgs(nil)
		s.root.SetOut(nil)
		s.root.SetErr(nil)
	}()
	if err := s.root.ExecuteContext(ctx); err != nil {
		return "", fmt.Errorf("%s: %w (stderr: %s)", id, err, strings.TrimSpace(errBuf.String()))
	}
	return buf.String(), nil
}
