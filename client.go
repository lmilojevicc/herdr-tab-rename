package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
)

type rpcResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcCall sends a single newline-delimited JSON-RPC request over a short-lived
// unix-socket connection and returns the "result" field of the response.
// Herdr's server closes the socket after answering one request.
func rpcCall(socket, method string, params any) (json.RawMessage, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", socket, err)
	}
	defer conn.Close()

	body, err := json.Marshal(map[string]any{"id": "q", "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	if _, err := conn.Write(body); err != nil {
		return nil, err
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return nil, err
	}

	var resp rpcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}
