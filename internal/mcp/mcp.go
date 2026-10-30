// Package mcp serves oryn's project knowledge to AI agents over the Model
// Context Protocol: newline-delimited JSON-RPC on stdin and stdout. Tools only
// read, apart from claims, which touch nothing but oryn's own database. There
// is deliberately no tool that deploys, deletes, pushes or starts anything.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const protocol = "2025-06-18"

// Tool is one callable. Its result is sent as JSON text.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Run         func(args json.RawMessage) (any, error)
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve answers requests until in closes.
func Serve(in io.Reader, out io.Writer, version string, tools []Tool) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			if err := enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			continue // a notification, which gets no reply
		}
		res := response{JSONRPC: "2.0", ID: req.ID}
		res.Result, res.Error = handle(req, version, tools)
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	return sc.Err()
}

func handle(req request, version string, tools []Tool) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		v := p.ProtocolVersion
		if v == "" {
			v = protocol
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "oryn", "version": version},
			"instructions":    "Call project_context first in a project: it gives the stack, commands, layout, CI, deploys and other agents, so you don't need to explore. Claim files before large edits.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := make([]map[string]any, len(tools))
		for i, t := range tools {
			list[i] = map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Schema}
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		for _, t := range tools {
			if t.Name == p.Name {
				return call(t, p.Arguments), nil
			}
		}
		return nil, &rpcError{-32602, fmt.Sprintf("unknown tool %s", p.Name)}
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func call(t Tool, args json.RawMessage) map[string]any {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	out, err := t.Run(args)
	if err != nil {
		return result(err.Error(), true)
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		return result(err.Error(), true)
	}
	return result(string(b), false)
}

func result(text string, isErr bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}
}
