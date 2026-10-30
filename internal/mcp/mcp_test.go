package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func serve(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	tools := []Tool{
		{"echo", "says hi", map[string]any{"type": "object"}, func(a json.RawMessage) (any, error) { return map[string]string{"got": string(a)}, nil }},
		{"fail", "breaks", map[string]any{"type": "object"}, func(json.RawMessage) (any, error) { return nil, errors.New("nope") }},
	}
	var out bytes.Buffer
	if err := Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, "1.2.3", tools); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		res = append(res, m)
	}
	return res
}

func TestHandshake(t *testing.T) {
	r := serve(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	if len(r) != 3 {
		t.Fatalf("notifications get no reply: %+v", r)
	}
	init := r[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2024-11-05" || init["serverInfo"].(map[string]any)["version"] != "1.2.3" {
		t.Errorf("init: %+v", init)
	}
	if tools := r[2]["result"].(map[string]any)["tools"].([]any); len(tools) != 2 || tools[0].(map[string]any)["inputSchema"] == nil {
		t.Errorf("tools: %+v", tools)
	}
}

func TestCalls(t *testing.T) {
	r := serve(t,
		`{"jsonrpc":"2.0","id":"a","method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fail"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"missing"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"nope"}`,
		`not json`)
	if r[0]["id"] != "a" {
		t.Errorf("id: %v", r[0]["id"])
	}
	first := r[0]["result"].(map[string]any)
	if first["isError"] != false || !strings.Contains(first["content"].([]any)[0].(map[string]any)["text"].(string), `\"x\":1`) {
		t.Errorf("echo: %+v", first)
	}
	if r[1]["result"].(map[string]any)["isError"] != true {
		t.Errorf("a failing tool reports isError: %+v", r[1])
	}
	for i, code := range map[int]float64{2: -32602, 3: -32601, 4: -32700} {
		if r[i]["error"].(map[string]any)["code"] != code {
			t.Errorf("reply %d: %+v", i, r[i])
		}
	}
}
