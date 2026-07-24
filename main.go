package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"context"
	"gopkg.in/yaml.v3"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

//go:embed static/*
var staticFS embed.FS

var (
	stubsCatalog      map[string]interface{}
	stubsCatalogMutex sync.Mutex
	stateMutex        sync.Mutex
)

func initCatalog() {
	stubsCatalog = map[string]interface{}{
		"http": map[string]interface{}{
			"name":         "HTTP Mock Server",
			"protocol":     "http",
			"default_port": 80,
			"description":  "HTTP Mock Server that returns a custom JSON response body and status code. Perfect for mocking authentication endpoints, third-party webhook receivers, or upstream microservices.",
			"code": `import http.server, socketserver
class Handler(http.server.BaseHTTPRequestHandler):
    def handle_request(self):
        self.send_response({status})
        self.send_header('Content-Type', 'application/json')
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS, PUT, DELETE')
        self.send_header('Access-Control-Allow-Headers', '*')
        self.end_headers()
        self.wfile.write(b'''{body}''')
    def do_GET(self): self.handle_request()
    def do_POST(self): self.handle_request()
    def do_PUT(self): self.handle_request()
    def do_DELETE(self): self.handle_request()
    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS, PUT, DELETE')
        self.send_header('Access-Control-Allow-Headers', '*')
        self.end_headers()
server = socketserver.TCPServer(('0.0.0.0', {port}), Handler)
print('Mock HTTP server running on port {port}...')
server.serve_forever()`,
		},
		"postgres": map[string]interface{}{
			"name":         "PostgreSQL Database",
			"protocol":     "postgres",
			"default_port": 5432,
			"description":  "Intercepts client-side SSLRequest queries, negotiates unencrypted startup, and replies with AuthenticationOK and ReadyForQuery packets. Keeps connection pool sockets alive.",
			"code": `import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', {port}))
s.listen(5)
print('Mock PostgreSQL stub running on port {port}...')
while True:
    try:
        c, a = s.accept()
        if {log_handshake}:
            print('Connection accepted from:', a)
        d = c.recv(1024)
        if d and len(d) == 8 and d[4:8] == b'\x04\xd2\x16\x2f':
            if {log_handshake}:
                print('Received SSLRequest, declining...')
            c.sendall(b'N')
            d = c.recv(1024)
        if {log_handshake} and d:
            print('Received StartupMessage:', d)
        c.sendall(b'R\x00\x00\x00\x08\x00\x00\x00\x00Z\x00\x00\x00\x05I')
        while True:
            d = c.recv(1024)
            if not d: break
            if d[0:1] == b'Q':
                c.sendall(b'I\x00\x00\x00\x04Z\x00\x00\x00\x05I')
            elif d[0:1] == b'X':
                break
        c.close()
    except Exception as e:
        print(e)`,
		},
		"mysql": map[string]interface{}{
			"name":         "MySQL / MariaDB",
			"protocol":     "mysql",
			"default_port": 3306,
			"description":  "Implements basic MySQL protocol Version 10 handshake and logs auth payload sequences to keep client adapters connected.",
			"code": `import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', {port}))
s.listen(5)
print('Mock MySQL stub running on port {port}...')
while True:
    try:
        c, a = s.accept()
        if {log_handshake}:
            print('Connection accepted from:', a)
        body = b'\x0a8.0.32-mock\x00\x01\x00\x00\x00\x01\x02\x03\x04\x05\x06\x07\x08\x00\x26\x82\x21\x02\x00\x0f\x80\x15\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x00mysql_native_password\x00'
        header = bytes([len(body) & 0xff, (len(body) >> 8) & 0xff, 0, 0])
        c.sendall(header + body)
        resp = c.recv(1024)
        if {log_handshake} and resp:
            print('Received login request from client')
        ok_body = b'\x00\x00\x00\x02\x00\x00\x00'
        ok_header = bytes([len(ok_body) & 0xff, 0, 0, 2])
        c.sendall(ok_header + ok_body)
        while True:
            d = c.recv(1024)
            if not d or len(d) < 5: break
            seq = d[3]
            cmd = d[4]
            if cmd == 0x01: # COM_QUIT
                break
            resp_seq = (seq + 1) & 0xff
            resp_body = b'\x00\x00\x00\x02\x00\x00\x00'
            resp_hdr = bytes([len(resp_body) & 0xff, 0, 0, resp_seq])
            c.sendall(resp_hdr + resp_body)
        c.close()
    except Exception as e:
        print(e)`,
		},
		"redis": map[string]interface{}{
			"name":         "Redis RESP Cache",
			"protocol":     "redis",
			"default_port": 6379,
			"description":  "Simulates Redis RESP command engine. Replies to PING with +PONG, SELECT/AUTH/SET with +OK, and GET requests with null string ($-1) to satisfy cache connection checks.",
			"code": `import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', {port}))
s.listen(5)
print('Mock Redis stub running on port {port}...')
while True:
    try:
        c, a = s.accept()
        if {log_handshake}:
            print('Connection accepted from:', a)
        while True:
            d = c.recv(1024)
            if not d: break
            d_upper = d.upper()
            if {log_handshake}:
                print('Received command:', d)
            if b'PING' in d_upper:
                c.sendall(b'+PONG\r\n')
            elif b'AUTH' in d_upper or b'SELECT' in d_upper or b'SET' in d_upper or b'FLUSHALL' in d_upper:
                c.sendall(b'+OK\r\n')
            elif b'GET' in d_upper:
                c.sendall(b'$-1\r\n')
            elif b'COMMAND' in d_upper:
                c.sendall(b'*0\r\n')
            elif b'INFO' in d_upper:
                info = b"# Server\r\nredis_version:7.0.0\r\n"
                c.sendall(bytes(f"${len(info)}\r\n", 'utf-8') + info + b"\r\n")
            else:
                c.sendall(b'+OK\r\n')
        c.close()
    except Exception as e:
        print(e)`,
		},
		"mongodb": map[string]interface{}{
			"name":         "MongoDB Wire Protocol",
			"protocol":     "mongodb",
			"default_port": 27017,
			"description":  "Parses MongoDB wire messages, matches OP_QUERY or OP_MSG requests, and responds to hello/isMaster command payloads with pre-encoded BSON data to satisfy driver checks.",
			"code": `import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', {port}))
s.listen(5)
print('Mock MongoDB stub running on port {port}...')
bson_doc = b'\x4c\x00\x00\x00\x08ismaster\x00\x01\x10maxBsonObjectSize\x00\x00\x00\x00\x01\x10maxMessageSizeBytes\x00\x00\x6c\xdc\x02\x01ok\x00\x00\x00\x00\x00\x00\x00\xf0\x3f\x00'
while True:
    try:
        c, a = s.accept()
        if {log_handshake}:
            print('Connection accepted from:', a)
        while True:
            hdr = c.recv(16)
            if not hdr or len(hdr) < 16: break
            msg_len = int.from_bytes(hdr[0:4], 'little')
            req_id = int.from_bytes(hdr[4:8], 'little')
            opcode = int.from_bytes(hdr[12:16], 'little')
            if msg_len > 16:
                c.recv(msg_len - 16)
            if {log_handshake}:
                print('Received MongoDB packet opcode:', opcode)
            if opcode == 2013:
                total_len = 21 + len(bson_doc)
                resp = bytes([total_len & 0xff, (total_len >> 8) & 0xff, (total_len >> 16) & 0xff, (total_len >> 24) & 0xff]) + b'\x00\x00\x00\x00' + bytes([req_id & 0xff, (req_id >> 8) & 0xff, (req_id >> 16) & 0xff, (req_id >> 24) & 0xff]) + b'\xdd\x07\x00\x00\x00\x00\x00\x00\x00' + bson_doc
                c.sendall(resp)
            else:
                total_len = 36 + len(bson_doc)
                resp = bytes([total_len & 0xff, (total_len >> 8) & 0xff, 0, 0]) + b'\x00\x00\x00\x00' + bytes([req_id & 0xff, (req_id >> 8) & 0xff, (req_id >> 16) & 0xff, (req_id >> 24) & 0xff]) + b'\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00' + bson_doc
                c.sendall(resp)
        c.close()
    except Exception as e:
        print(e)`,
		},
		"rabbitmq": map[string]interface{}{
			"name":         "RabbitMQ / AMQP Stub",
			"protocol":     "rabbitmq",
			"default_port": 5672,
			"description":  "Reads AMQP protocol headers (AMQP\x00\x00\x09\x01) and completes RabbitMQ connection handshake (Connection.Start -> Tune -> Open) using AMQP binary frames.",
			"code": `import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', {port}))
s.listen(5)
print('Mock RabbitMQ stub running on port {port}...')
while True:
    try:
        c, a = s.accept()
        if {log_handshake}:
            print('Connection accepted from:', a)
        hdr = c.recv(8)
        if not hdr or hdr[0:4] != b'AMQP':
            c.close()
            continue
        if {log_handshake}:
            print('Received AMQP header:', hdr)
        c.sendall(b'\x01\x00\x00\x00\x00\x00\x1c\x00\x0a\x00\x0a\x00\x09\x00\x00\x00\x00\x00\x00\x00\x05PLAIN\x00\x00\x00\x05en_US\xce')
        d = c.recv(1024)
        c.sendall(b'\x01\x00\x00\x00\x00\x00\x0c\x00\x0a\x00\x1e\x00\x00\x00\x02\x00\x00\x00\x00\xce')
        d = c.recv(1024)
        c.sendall(b'\x01\x00\x00\x00\x00\x00\x05\x00\x0a\x00\x29\x00\xce')
        while True:
            d = c.recv(1024)
            if not d: break
        c.close()
    except Exception as e:
        print(e)`,
		},
		"tcp": map[string]interface{}{
			"name":         "Generic TCP Logger",
			"protocol":     "tcp",
			"default_port": 8080,
			"description":  "A fallback TCP socket listener that binds to the target port, sends a 'Ready\n' handshake (customizable), and dumps all incoming text/binary data to stdout.",
			"code": `import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', {port}))
s.listen(5)
print('Mock TCP stub running on port {port}...')
while True:
    try:
        c, a = s.accept()
        if {log_handshake}:
            print('Connection accepted from:', a)
            c.sendall(b'Ready\n')
        d = c.recv(1024)
        if d and {log_handshake}:
            print('Received payload:', d)
        c.close()
    except Exception as e:
        print(e)`,
		},
		"llm_openai": map[string]interface{}{
			"name":         "LLM REST Mocking (OpenAI)",
			"protocol":     "llm_openai",
			"default_port": 443,
			"description":  "Intercepts REST API calls to api.openai.com (v1/chat/completions) and returns mock completion responses, supporting streaming (SSE) and rate limits emulation.",
			"code":         `Mock OpenAI stub running on port {port}...`,
		},
		"llm_anthropic": map[string]interface{}{
			"name":         "LLM REST Mocking (Anthropic)",
			"protocol":     "llm_anthropic",
			"default_port": 443,
			"description":  "Intercepts REST API calls to api.anthropic.com (v1/messages) and returns mock message completions, supporting streaming (SSE) and rate limits emulation.",
			"code":         `Mock Anthropic stub running on port {port}...`,
		},
		"llm_gemini": map[string]interface{}{
			"name":         "LLM REST Mocking (Gemini)",
			"protocol":     "llm_gemini",
			"default_port": 443,
			"description":  "Intercepts REST API calls to Gemini (generateContent) and returns mock text outputs.",
			"code":         `Mock Gemini stub running on port {port}...`,
		},
		"vectordb": map[string]interface{}{
			"name":         "Vector Database Mocking",
			"protocol":     "vectordb",
			"default_port": 443,
			"description":  "Intercepts vector database query endpoints and simulates similarity search matches and metric distances.",
			"code":         `Mock Vector DB stub running on port {port}...`,
		},
	}
}

func loadStubsLibrary() map[string]interface{} {
	stubsCatalogMutex.Lock()
	defer stubsCatalogMutex.Unlock()

	catalog := map[string]interface{}{}
	for k, v := range stubsCatalog {
		catalog[k] = v
	}

	if _, err := os.Stat(LIBRARY_PATH); err == nil {
		body, err := os.ReadFile(LIBRARY_PATH)
		if err == nil {
			var custom map[string]interface{}
			if err := json.Unmarshal(body, &custom); err == nil {
				for k, v := range custom {
					if _, exists := stubsCatalog[k]; !exists {
						catalog[k] = v
					}
				}
			}
		}
	}
	return catalog
}

func saveCustomStubs(catalog map[string]interface{}) {
	custom := map[string]interface{}{}
	for k, v := range catalog {
		if _, exists := stubsCatalog[k]; !exists {
			custom[k] = v
		}
	}

	body, err := json.MarshalIndent(custom, "", "  ")
	if err == nil {
		_ = os.WriteFile(LIBRARY_PATH, body, 0644)
	}
}

// Enable CORS middleware
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || origin == "http://localhost:11800" || origin == "http://127.0.0.1:11800" {
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func handleGetCompose(w http.ResponseWriter, r *http.Request) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	// Dynamically reload global config to see if active workspace was changed by CLI (e.g. mockdock init)
	gcfg, err := readGlobalConfig()
	if err == nil && gcfg != nil && gcfg.ActiveWorkspace != "" && gcfg.ActiveWorkspace != WORKSPACE_ID {
		if ws, ok := gcfg.Workspaces[gcfg.ActiveWorkspace]; ok {
			WORKSPACE_DIR = ws.Path
			updateActivePaths(ws.ID, "default")
			loadSourcesFromDisk()
		}
	}

	// Sort services alphabetically by name to prevent randomized JSON response ordering
	var serviceKeys []string
	for k := range state.Services {
		serviceKeys = append(serviceKeys, k)
	}
	sort.Strings(serviceKeys)
	
	servicesList := []ServiceState{}
	for _, k := range serviceKeys {
		servicesList = append(servicesList, state.Services[k])
	}

	// Sort external services alphabetically by name
	var externalServiceKeys []string
	for k := range state.ExternalServices {
		externalServiceKeys = append(externalServiceKeys, k)
	}
	sort.Strings(externalServiceKeys)
	
	externalServicesList := []ExternalServiceState{}
	for _, k := range externalServiceKeys {
		externalServicesList = append(externalServicesList, state.ExternalServices[k])
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"services":          servicesList,
		"external_services": externalServicesList,
		"networks":          state.Networks,
		"raw_compose":       state.RawCompose,
		"audit":             state.AuditResults,
		"project_label":     state.ProjectLabel,
		"sources":           state.Sources,
		"workspace_id":      WORKSPACE_ID,
		"active_profile":    ACTIVE_PROFILE,
		"compose_error":     state.ComposeError,
	})
}

func handleUpdateCompose(w http.ResponseWriter, r *http.Request) {
	var body struct {
		YamlContent string `json:"yaml_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	res := initializeStateFromCompose(body.YamlContent)
	if errMsg, ok := res["error"]; ok {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": fmt.Sprintf("%v", errMsg)})
		return
	}

	// Persist
	err := os.WriteFile(COMPOSE_PATH, []byte(body.YamlContent), 0644)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": fmt.Sprintf("Failed to save compose file: %v", err)})
		return
	}

	servicesList := []interface{}{}
	for _, s := range state.Services {
		servicesList = append(servicesList, s)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"services": servicesList,
		"audit":    state.AuditResults,
	})
}

func handleUpdateSources(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectLabel string       `json:"project_label"`
		Sources      []SourceItem `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	state.ProjectLabel = body.ProjectLabel
	state.Sources = body.Sources

	res := loadSourcesAndMerge()
	if _, ok := res["error"]; ok {
		jsonResponse(w, http.StatusBadRequest, res)
		return
	}

	// Persist sources
	sourcesBody, err := json.MarshalIndent(map[string]interface{}{
		"project_label": state.ProjectLabel,
		"sources":       state.Sources,
	}, "", "  ")
	if err == nil {
		_ = os.WriteFile(SOURCES_PATH, sourcesBody, 0644)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"project_label": state.ProjectLabel,
		"sources":       state.Sources,
	})
}

func handleUploadSourceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"detail": "Method not allowed"})
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10MB max
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Failed to parse form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Missing file in request"})
		return
	}
	defer file.Close()

	// Ensure filename is secure
	filename := filepath.Base(header.Filename)
	if filename == "." || filename == "/" || filename == ".." {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid filename"})
		return
	}

	// Target directory (Zero Git Pollution)
	// Target directory (mockdock/sources inside repository)
	sourcesDir := filepath.Join(WORKSPACE_DIR, "mockdock", "sources")
	err = os.MkdirAll(sourcesDir, 0755)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": "Failed to create directory"})
		return
	}

	destPath := filepath.Join(sourcesDir, filename)
	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": "Failed to create destination file"})
		return
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, file)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": "Failed to write file"})
		return
	}

	relativePath := "mockdock/sources/" + filename
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"path":    relativePath,
	})
}

type ProfileMetadata struct {
	ProjectLabel     string                          `json:"project_label" yaml:"project_label"`
	Services         map[string]ServiceState         `json:"services" yaml:"services"`
	ExternalServices map[string]ExternalServiceState `json:"external_services" yaml:"external_services"`
	Sources          []SourceItem                    `json:"sources" yaml:"sources"`
	ActiveStubs      []string                        `json:"active_stubs" yaml:"active_stubs"`
	CustomStubs      map[string]interface{}          `json:"custom_stubs" yaml:"custom_stubs"`
}

type ProfileFile struct {
	Version          string                 `yaml:"version"`
	XMockdockProfile ProfileMetadata        `yaml:"x-mockdock-profile"`
	Services         map[string]interface{} `yaml:"services"`
}

func handleAddUniverse(w http.ResponseWriter, r *http.Request) {
	var name, pathVal, urlVal string
	var fileData []byte
	var fileHeaderName string

	// Check if request is multipart form
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		err := r.ParseMultipartForm(10 << 20) // 10MB max
		if err == nil {
			name = r.FormValue("name")
			pathVal = r.FormValue("path")
			urlVal = r.FormValue("url")
			
			file, header, fileErr := r.FormFile("file")
			if fileErr == nil {
				defer file.Close()
				fileHeaderName = header.Filename
				fileData, _ = io.ReadAll(file)
			}
		}
	} else {
		var body struct {
			Name string `json:"name"`
			Path string `json:"path"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			name = body.Name
			pathVal = body.Path
			urlVal = body.URL
		}
	}

	name = strings.TrimSpace(name)
	pathVal = strings.TrimSpace(pathVal)
	if name == "" || pathVal == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Name and Path are required"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	err := addUniverse(name, pathVal, urlVal, fileData, fileHeaderName)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"workspace_id": WORKSPACE_ID,
	})
}

func handleRemoveUniverse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	gcfg, err := readGlobalConfig()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	if _, exists := gcfg.Workspaces[body.WorkspaceID]; !exists {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Universe does not exist"})
		return
	}

	delete(gcfg.Workspaces, body.WorkspaceID)
	err = writeGlobalConfig(gcfg)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	// If we removed the currently active workspace, fall back to "mockdock" default
	if WORKSPACE_ID == body.WorkspaceID {
		WORKSPACE_ID = "mockdock"
		cwd, _ := os.Getwd()
		WORKSPACE_DIR = cwd // default CWD
		updateActivePaths("mockdock", "default")
		_ = loadWorkspaceProfile("default")
		
		gcfg.ActiveWorkspace = "mockdock"
		_ = writeGlobalConfig(gcfg)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	gcfg, err := readGlobalConfig()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	workspaces := []WorkspaceMetadata{}
	for _, ws := range gcfg.Workspaces {
		// Populate sources list
		universeDir := getUniverseDir(ws.ID)
		sourcesPath := filepath.Join(universeDir, "sources.json")
		var sourcesList []string
		if body, err := os.ReadFile(sourcesPath); err == nil {
			var srcData struct {
				Sources []SourceItem `json:"sources"`
			}
			if err := json.Unmarshal(body, &srcData); err == nil {
				for _, s := range srcData.Sources {
					if s.Enabled {
						sourcesList = append(sourcesList, filepath.Base(s.Value))
					}
				}
			}
		}
		if len(sourcesList) == 0 {
			sourcesList = []string{"None"}
		}
		ws.Sources = sourcesList

		// Populate profiles list
		profilesDir := filepath.Join(universeDir, "profiles")
		var profilesList []string
		if files, err := os.ReadDir(profilesDir); err == nil {
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".yaml") {
					profilesList = append(profilesList, strings.TrimSuffix(f.Name(), ".yaml"))
				}
			}
		}
		localSourcesDir := filepath.Join(ws.Path, "mockdock", "sources")
		if files, err := os.ReadDir(localSourcesDir); err == nil {
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), "-docker-compose.yaml") {
					profilesList = append(profilesList, strings.TrimSuffix(f.Name(), "-docker-compose.yaml"))
				}
			}
		}
		if len(profilesList) == 0 {
			profilesList = []string{"default"}
		}
		ws.Profiles = profilesList

		// Populate active profile metadata
		if ws.ID == WORKSPACE_ID {
			ws.ActiveProfile = state.ActiveProfile
		} else {
			ws.ActiveProfile = "default"
		}

		workspaces = append(workspaces, ws)
	}

	// Sort workspaces alphabetically
	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].ID < workspaces[j].ID
	})

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"workspaces": workspaces,
	})
}

func handleSelectWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	gcfg, err := readGlobalConfig()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	ws, exists := gcfg.Workspaces[body.WorkspaceID]
	if !exists {
		jsonResponse(w, http.StatusNotFound, map[string]string{"detail": "Workspace not found"})
		return
	}

	if !universeHasSource(ws.ID, ws.Path) {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Cannot activate universe: no configuration sources found. Please place a compose file in your project or mockdock/sources/ directory first."})
		return
	}

	// Automatically stop the previous active stack to release ports/networks
	prevID := gcfg.ActiveWorkspace
	if prevID != "" && prevID != ws.ID {
		if prevWs, ok := gcfg.Workspaces[prevID]; ok {
			prevCompose := filepath.Join(getUniverseDir(prevID), "source-compose.yaml")
			prevMocked := filepath.Join(getUniverseDir(prevID), "mocked-compose.yaml")
			if _, errC := os.Stat(prevCompose); errC == nil {
				args := []string{"compose", "--project-directory", prevWs.Path, "-f", prevCompose}
				if _, errM := os.Stat(prevMocked); errM == nil {
					args = append(args, "-f", prevMocked)
				}
				args = append(args, "down")
				cmd := exec.Command("docker", args...)
				cmd.Dir = prevWs.Path
				_ = cmd.Run()
			}
		}
	}

	// Persist active workspace in global config
	gcfg.ActiveWorkspace = ws.ID
	err = writeGlobalConfig(gcfg)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	// Shift daemon context to this workspace path & ID
	WORKSPACE_DIR = ws.Path
	updateActivePaths(ws.ID, "default")

	// Read workspace default profile
	err = loadWorkspaceProfile("default")
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": fmt.Sprintf("Failed to load default profile: %v", err)})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"workspace_id": ws.ID,
		"workspace":    ws,
	})
}

func handleListProfiles(w http.ResponseWriter, r *http.Request) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	universeDir := getUniverseDir(WORKSPACE_ID)
	profilesDir := filepath.Join(universeDir, "profiles")
	_ = os.MkdirAll(profilesDir, 0755)

	files, err := os.ReadDir(profilesDir)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	profiles := []string{}
	for _, f := range files {
		if !f.IsDir() && (strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml")) {
			name := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
			profiles = append(profiles, name)
		}
	}

	sort.Strings(profiles)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"profiles": profiles,
	})
}

func saveProfileInternalLocked(profileName string) error {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return fmt.Errorf("profile name is required")
	}

	servicesOverrides := map[string]interface{}{}
	for name, stateSvc := range state.Services {
		if stateSvc.Mocked {
			svcOverride := map[string]interface{}{}
			protocol := stateSvc.StubConfig.Protocol
			defaultPort := 80
			switch protocol {
			case "postgres":
				defaultPort = 5432
			case "mysql":
				defaultPort = 3306
			case "redis":
				defaultPort = 6379
			case "mongodb":
				defaultPort = 27017
			case "rabbitmq":
				defaultPort = 5672
			}
			containerPort := defaultPort
			if len(stateSvc.Ports) > 0 {
				pStr := stateSvc.Ports[0]
				if strings.Contains(pStr, ":") {
					parts := strings.SplitN(pStr, ":", 2)
					containerPort, _ = strconv.Atoi(parts[1])
				} else {
					containerPort, _ = strconv.Atoi(pStr)
				}
			}

			svcOverride["image"] = "ghcr.io/mockdockapp/mockdock:latest"
			svcOverride["build"] = nil
			svcOverride["command"] = []string{
				"mockdock", "stub",
				"--protocol", protocol,
				"--port", strconv.Itoa(containerPort),
				"--http-status", strconv.Itoa(stateSvc.StubConfig.HTTPStatus),
				"--response-body", stateSvc.StubConfig.ResponseBody,
				"--log-handshake=" + strconv.FormatBool(stateSvc.StubConfig.TCPLogHandshake),
				"--latency", strconv.Itoa(stateSvc.ChaosConfig.LatencyMS),
				"--loss", strconv.Itoa(stateSvc.ChaosConfig.PacketLossPct),
				"--script", stateSvc.StubConfig.Script,
				"--http-crud=" + strconv.FormatBool(stateSvc.StubConfig.HTTPCRUD),
				"--sqlite-enabled=" + strconv.FormatBool(stateSvc.StubConfig.SQLiteEnabled),
				"--llm-provider", stateSvc.StubConfig.LLMProvider,
				"--llm-model", stateSvc.StubConfig.LLMModel,
				"--llm-stream=" + strconv.FormatBool(stateSvc.StubConfig.LLMStream),
				"--llm-rate-limit=" + strconv.FormatBool(stateSvc.StubConfig.LLMRateLimit),
				"--llm-tokens-limit", strconv.Itoa(stateSvc.StubConfig.LLMTokensLimit),
				"--llm-reqs-limit", strconv.Itoa(stateSvc.StubConfig.LLMReqsLimit),
				"--llm-ttft", strconv.Itoa(stateSvc.StubConfig.LLMTtftMs),
				"--llm-token-delay", strconv.Itoa(stateSvc.StubConfig.LLMTokenDelayMs),
				"--chaos-jitter", strconv.Itoa(stateSvc.ChaosConfig.LatencyJitter),
				"--chaos-bandwidth", strconv.Itoa(stateSvc.ChaosConfig.Bandwidth),
				"--chaos-dns-status", stateSvc.ChaosConfig.DnsStatus,
				"--chaos-dns-delay", strconv.Itoa(stateSvc.ChaosConfig.DnsDelayMs),
				"--chaos-truncated=" + strconv.FormatBool(stateSvc.ChaosConfig.TruncatedResp),
				"--chaos-cpu", strconv.Itoa(stateSvc.ChaosConfig.CpuSpikePct),
				"--chaos-mem", strconv.Itoa(stateSvc.ChaosConfig.MemSpikeMb),
			}
			svcOverride["cap_add"] = []string{"NET_ADMIN"}
			svcOverride["volumes"] = []interface{}{}
			svcOverride["healthcheck"] = map[string]interface{}{
				"disable": true,
			}

			if stateSvc.ChaosConfig.Disconnected {
				svcOverride["networks"] = []string{}
				svcOverride["ports"] = []string{}
			} else {
				if len(stateSvc.Ports) > 0 {
					svcOverride["ports"] = stateSvc.Ports
				}
			}

			servicesOverrides[name] = svcOverride
		}

		// Inject extra_hosts dynamic mutations if SaaS is intercepted
		var activeDomains []string
		envList := []string{}
		if stateSvc.Environment != nil {
			if m, ok := stateSvc.Environment.(map[string]interface{}); ok {
				for k, v := range m {
					envList = append(envList, fmt.Sprintf("%s=%v", k, v))
				}
			} else if l, ok := stateSvc.Environment.([]interface{}); ok {
				for _, item := range l {
					envList = append(envList, fmt.Sprintf("%v", item))
				}
			}
		}

		for _, env := range envList {
			if strings.Contains(env, "=") {
				parts := strings.SplitN(env, "=", 2)
				val := parts[1]
				domains := extractDomains(val)
				for _, dom := range domains {
					if extSvc, exists := state.ExternalServices[dom]; exists {
						isMockAllowed := extSvc.Mocked
						if !IS_RAD_ACTIVE && extSvc.Mocked {
							protoCounts := map[string]int{}
							for _, s := range state.Services {
								if s.Mocked {
									protoCounts[strings.ToLower(s.StubConfig.Protocol)]++
								}
							}
							for _, es := range state.ExternalServices {
								if es.Mocked {
									protoCounts[strings.ToLower(es.StubConfig.Protocol)]++
								}
							}
							proto := strings.ToLower(extSvc.StubConfig.Protocol)
							isBuiltInProto := func(p string) bool {
								return p == "http" || p == "postgres" || p == "mysql" || p == "redis" || p == "mongodb" || p == "rabbitmq"
							}
							if !isBuiltInProto(proto) {
								isMockAllowed = false
							} else if protoCounts[proto] > 1 {
								isMockAllowed = false
							}
						}
						if isMockAllowed || extSvc.ChaosConfig.LatencyMS > 0 || extSvc.ChaosConfig.PacketLossPct > 0 || extSvc.ChaosConfig.Disconnected {
							activeDomains = append(activeDomains, dom)
						}
					}
				}
			}
		}

		if len(activeDomains) > 0 {
			var extraHosts []string
			for _, dom := range activeDomains {
				extraHosts = append(extraHosts, fmt.Sprintf("%s:host-gateway", dom))
			}
			
			svcOverride, exists := servicesOverrides[name]
			if !exists {
				svcOverride = map[string]interface{}{}
			}
			svcOverride.(map[string]interface{})["extra_hosts"] = extraHosts
			svcOverride.(map[string]interface{})["cap_add"] = []string{"NET_ADMIN"}
			servicesOverrides[name] = svcOverride
		}
	}

	// Collect active stubs configuration
	activeStubsPath := filepath.Join(getUniverseDir(WORKSPACE_ID), "active_stubs.json")
	var activeStubsList []string
	if regBytes, err := os.ReadFile(activeStubsPath); err == nil {
		_ = json.Unmarshal(regBytes, &activeStubsList)
	}

	// Collect custom handshake stubs from library
	customCatalog := make(map[string]interface{})
	fullCatalog := loadStubsLibrary()
	for k, v := range fullCatalog {
		if _, exists := stubsCatalog[k]; !exists {
			customCatalog[k] = v
		}
	}

	profileFile := ProfileFile{
		Version: "3.8",
		XMockdockProfile: ProfileMetadata{
			ProjectLabel:     state.ProjectLabel,
			Services:         state.Services,
			ExternalServices: state.ExternalServices,
			Sources:          state.Sources,
			ActiveStubs:      activeStubsList,
			CustomStubs:      customCatalog,
		},
		Services: servicesOverrides,
	}

	universeDir := getUniverseDir(WORKSPACE_ID)
	profilePath := filepath.Join(universeDir, "profiles", profileName+".yaml")

	yamlBody, err := yaml.Marshal(profileFile)
	if err != nil {
		return err
	}

	yamlBodyStr := string(yamlBody)
	yamlBodyStr = strings.ReplaceAll(yamlBodyStr, "build: null", "build: !reset null")
	yamlBody = []byte(yamlBodyStr)

	err = os.WriteFile(profilePath, yamlBody, 0644)
	if err != nil {
		return err
	}

	if WORKSPACE_DIR != "" {
		localSourcesDir := filepath.Join(WORKSPACE_DIR, "mockdock", "sources")
		_ = os.MkdirAll(localSourcesDir, 0755)
		localProfilePath := filepath.Join(localSourcesDir, profileName+"-docker-compose.yaml")
		_ = os.WriteFile(localProfilePath, yamlBody, 0644)
	}

	return nil
}

func handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileName string `json:"profile_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	profileName := strings.TrimSpace(body.ProfileName)
	if profileName == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Profile name is required"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	err := saveProfileInternalLocked(profileName)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": fmt.Sprintf("Failed to save profile: %v", err)})
		return
	}

	ACTIVE_PROFILE = profileName
	state.ActiveProfile = ACTIVE_PROFILE

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"profile": profileName,
	})
}

func handleLoadProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileName string `json:"profile_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	profileName := strings.TrimSpace(body.ProfileName)
	if profileName == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Profile name is required"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	// 1. Cleanly teardown any running stack tasks
	_, _, _ = runComposeDown()

	// 2. Load and merge profile content
	err := loadWorkspaceProfile(profileName)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}

	// 2b. Check for port conflicts before starting the stack
	ports, _ := getComposeHostPorts()
	conflicts, errCheck := checkPortsConflicts(ports)
	if errCheck == nil && len(conflicts) > 0 {
		hasUniverseConflict := false
		for _, conf := range conflicts {
			if conf.ConflictType == "universe" {
				hasUniverseConflict = true
				break
			}
		}
		if hasUniverseConflict {
			jsonResponse(w, http.StatusConflict, map[string]interface{}{
				"success":   false,
				"conflicts": conflicts,
			})
			return
		}
	}

	// Clean and populate port remappings
	portRemappings = make(map[int]int)
	state.PortRemappings = []PortRemapInfo{}

	for _, conf := range conflicts {
		if conf.ConflictType == "external" {
			freePort := findFreePort(conf.Port + 1)
			if freePort > 0 {
				portRemappings[conf.Port] = freePort
				state.PortRemappings = append(state.PortRemappings, PortRemapInfo{
					OriginalPort: conf.Port,
					RemappedPort: freePort,
				})
				log.Printf("[Port Manager] Port %d is occupied by an external host process. Re-mapped to %d.", conf.Port, freePort)
			}
		}
	}

	// Re-run export to apply labels and remapped ports
	exportMockCompose()

	// 3. Boot updated matrix
	stdout, stderr, err := runComposeUp(false)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"stdout":  stdout,
			"stderr":  stderr,
			"detail":  formatComposeError(stderr, err),
		})
		return
	}

	// Apply netem chaos configurations on Real services in background
	go func() {
		time.Sleep(3 * time.Second)
		stateMutex.Lock()
		defer stateMutex.Unlock()
		for name, svc := range state.Services {
			if !svc.Mocked && (svc.ChaosConfig.LatencyMS > 0 || svc.ChaosConfig.PacketLossPct > 0) {
				containerID, err := getContainerIDByService(name)
				if err == nil && containerID != "" {
					_ = applyTrafficControl(containerID, svc.ChaosConfig.LatencyMS, svc.ChaosConfig.PacketLossPct)
				}
			}
		}
	}()

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"profile": profileName,
		"stdout":  stdout,
		"stderr":  stderr,
	})
}

func handleToggleService(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServiceName string `json:"service_name"`
		Mocked      bool   `json:"mocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	svc, exists := state.Services[body.ServiceName]
	if !exists {
		extSvc, extExists := state.ExternalServices[body.ServiceName]
		if !extExists {
			jsonResponse(w, http.StatusNotFound, map[string]string{"detail": "Service not found"})
			return
		}
		extSvc.Mocked = body.Mocked
		state.ExternalServices[body.ServiceName] = extSvc

		runAudit()
		_ = saveProfileInternalLocked(ACTIVE_PROFILE)

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"service": extSvc,
		})
		return
	}

	svc.Mocked = body.Mocked
	state.Services[body.ServiceName] = svc

	runAudit()
	_ = saveProfileInternalLocked(ACTIVE_PROFILE)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"service": svc,
	})
}

func handleConfigureStub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServiceName     string `json:"service_name"`
		Protocol        string `json:"protocol"`
		HTTPStatus      int    `json:"http_status"`
		ResponseBody    string `json:"response_body"`
		TCPLogHandshake bool   `json:"tcp_log_handshake"`
		Script          string `json:"script"`
		HTTPCRUD        bool   `json:"http_crud"`
		SQLiteEnabled   bool   `json:"sqlite_enabled"`
		LLMProvider     string `json:"llm_provider"`
		LLMModel        string `json:"llm_model"`
		LLMStream       bool   `json:"llm_stream"`
		LLMRateLimit    bool   `json:"llm_rate_limit"`
		LLMTokensLimit  int    `json:"llm_tokens_limit"`
		LLMReqsLimit    int    `json:"llm_reqs_limit"`
		LLMTtftMs       int    `json:"llm_ttft_ms"`
		LLMTokenDelayMs int    `json:"llm_token_delay_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	svc, exists := state.Services[body.ServiceName]
	if !exists {
		extSvc, extExists := state.ExternalServices[body.ServiceName]
		if !extExists {
			jsonResponse(w, http.StatusNotFound, map[string]string{"detail": "Service not found"})
			return
		}

		extSvc.StubConfig = StubConfig{
			Protocol:        body.Protocol,
			HTTPStatus:      body.HTTPStatus,
			ResponseBody:    body.ResponseBody,
			TCPLogHandshake: body.TCPLogHandshake,
			Script:          body.Script,
			HTTPCRUD:        body.HTTPCRUD,
			SQLiteEnabled:   body.SQLiteEnabled,
			LLMProvider:     body.LLMProvider,
			LLMModel:        body.LLMModel,
			LLMStream:       body.LLMStream,
			LLMRateLimit:    body.LLMRateLimit,
			LLMTokensLimit:  body.LLMTokensLimit,
			LLMReqsLimit:    body.LLMReqsLimit,
			LLMTtftMs:       body.LLMTtftMs,
			LLMTokenDelayMs: body.LLMTokenDelayMs,
		}
		state.ExternalServices[body.ServiceName] = extSvc
		_ = saveProfileInternalLocked(ACTIVE_PROFILE)

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"service": extSvc,
		})
		return
	}

	svc.StubConfig = StubConfig{
		Protocol:        body.Protocol,
		HTTPStatus:      body.HTTPStatus,
		ResponseBody:    body.ResponseBody,
		TCPLogHandshake: body.TCPLogHandshake,
		Script:          body.Script,
		HTTPCRUD:        body.HTTPCRUD,
		SQLiteEnabled:   body.SQLiteEnabled,
		LLMProvider:     body.LLMProvider,
		LLMModel:        body.LLMModel,
		LLMStream:       body.LLMStream,
		LLMRateLimit:    body.LLMRateLimit,
		LLMTokensLimit:  body.LLMTokensLimit,
		LLMReqsLimit:    body.LLMReqsLimit,
		LLMTtftMs:       body.LLMTtftMs,
		LLMTokenDelayMs: body.LLMTokenDelayMs,
	}
	state.Services[body.ServiceName] = svc
	_ = saveProfileInternalLocked(ACTIVE_PROFILE)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"service": svc,
	})
}

func handleConfigureChaos(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServiceName   string `json:"service_name"`
		LatencyMS     int    `json:"latency_ms"`
		PacketLossPct int    `json:"packet_loss_pct"`
		Disconnected  bool   `json:"disconnected"`
		LatencyJitter int    `json:"latency_jitter"`
		Bandwidth     int    `json:"bandwidth"`
		DnsStatus     string `json:"dns_status"`
		DnsDelayMs    int    `json:"dns_delay_ms"`
		TruncatedResp bool   `json:"truncated_resp"`
		CpuSpikePct   int    `json:"cpu_spike_pct"`
		MemSpikeMb    int    `json:"mem_spike_mb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	svc, exists := state.Services[body.ServiceName]
	if !exists {
		extSvc, extExists := state.ExternalServices[body.ServiceName]
		if !extExists {
			jsonResponse(w, http.StatusNotFound, map[string]string{"detail": "Service not found"})
			return
		}
		extSvc.ChaosConfig = ChaosConfig{
			LatencyMS:     body.LatencyMS,
			PacketLossPct: body.PacketLossPct,
			Disconnected:  body.Disconnected,
			LatencyJitter: body.LatencyJitter,
			Bandwidth:     body.Bandwidth,
			DnsStatus:     body.DnsStatus,
			DnsDelayMs:    body.DnsDelayMs,
			TruncatedResp: body.TruncatedResp,
			CpuSpikePct:   body.CpuSpikePct,
			MemSpikeMb:    body.MemSpikeMb,
		}
		state.ExternalServices[body.ServiceName] = extSvc
		_ = saveProfileInternalLocked(ACTIVE_PROFILE)

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"service": extSvc,
		})
		return
	}

	svc.ChaosConfig = ChaosConfig{
		LatencyMS:     body.LatencyMS,
		PacketLossPct: body.PacketLossPct,
		Disconnected:  body.Disconnected,
		LatencyJitter: body.LatencyJitter,
		Bandwidth:     body.Bandwidth,
		DnsStatus:     body.DnsStatus,
		DnsDelayMs:    body.DnsDelayMs,
		TruncatedResp: body.TruncatedResp,
		CpuSpikePct:   body.CpuSpikePct,
		MemSpikeMb:    body.MemSpikeMb,
	}
	state.Services[body.ServiceName] = svc
	_ = saveProfileInternalLocked(ACTIVE_PROFILE)

	// Run in a background goroutine so the API response remains fast
	go func(svcName string, latency int, loss int, disconnected bool) {
		containerID, err := getContainerIDByService(svcName)
		if err != nil {
			log.Printf("[Chaos] Cannot apply tc rules to %s: %v", svcName, err)
			return
		}
		effectiveLoss := loss
		if disconnected {
			effectiveLoss = 100
		}
		err = applyTrafficControl(containerID, latency, effectiveLoss)
		if err != nil {
			log.Printf("[Chaos] Failed to apply tc rules to %s: %v", svcName, err)
		}
	}(body.ServiceName, body.LatencyMS, body.PacketLossPct, body.Disconnected)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"service": svc,
	})
}

func handleTestService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	stateMutex.Lock()
	svc, exists := state.Services[name]
	var extSvc ExternalServiceState
	var extExists bool
	if !exists {
		extSvc, extExists = state.ExternalServices[name]
	}
	stateMutex.Unlock()

	if !exists && !extExists {
		jsonResponse(w, http.StatusNotFound, map[string]string{"detail": "Service not found"})
		return
	}

	if extExists {
		status := extSvc.StubConfig.HTTPStatus
		if status == 0 {
			status = 200
		}
		body := extSvc.StubConfig.ResponseBody
		if body == "" {
			body = `{"status": "ok"}`
		}
		statusText := "OK"
		if status != 200 {
			statusText = "Custom Status"
		}
		
		curlCmd := fmt.Sprintf("curl -H \"Host: %s\" http://localhost", name)
		if !extSvc.Mocked {
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"mocked":       false,
				"status":       "External Service is bypassed (DNS clean)",
				"curl_command": curlCmd,
			})
			return
		}

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"mocked":       true,
			"protocol":     "HTTP (Intercepted)",
			"curl_command": curlCmd,
			"simulated_response": map[string]interface{}{
				"headers": map[string]string{
					"HTTP/1.1":               fmt.Sprintf("%d %s", status, statusText),
					"Content-Type":           "application/json",
					"X-Mockdock-Intercepted": "true",
				},
				"body": body,
			},
		})
		return
	}

	hostPort := "80"
	if len(svc.Ports) > 0 {
		pStr := svc.Ports[0]
		if strings.Contains(pStr, ":") {
			hostPort = strings.Split(pStr, ":")[0]
		} else {
			hostPort = pStr
		}
	}

	protocol := strings.ToLower(svc.StubConfig.Protocol)
	if protocol == "" {
		protocol = "http"
	}

	if !svc.Mocked {
		curlCmd := fmt.Sprintf("curl http://localhost:%s", hostPort)
		if protocol != "http" {
			curlCmd = fmt.Sprintf("nc localhost %s", hostPort)
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"mocked":       false,
			"status":       "Service is in Real mode (not mocked)",
			"curl_command": curlCmd,
		})
		return
	}

	if protocol == "http" {
		status := svc.StubConfig.HTTPStatus
		if status == 0 {
			status = 200
		}
		body := svc.StubConfig.ResponseBody
		if body == "" {
			body = `{"status": "ok"}`
		}
		statusText := "OK"
		if status != 200 {
			statusText = "Custom Status"
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"mocked":       true,
			"protocol":     "HTTP",
			"curl_command": fmt.Sprintf("curl -i http://localhost:%s", hostPort),
			"simulated_response": map[string]interface{}{
				"headers": map[string]string{
					"HTTP/1.1":     fmt.Sprintf("%d %s", status, statusText),
					"Content-Type": "application/json",
					"Server":       "MockDock-Stub",
				},
				"body": body,
			},
		})
	} else {
		logHandshake := svc.StubConfig.TCPLogHandshake
		handshakeMsg := "Sent READY handshake: 'Ready\\n'"
		if !logHandshake {
			handshakeMsg = "No handshake sent (silent mode)"
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"mocked":     true,
			"protocol":   "TCP",
			"nc_command": fmt.Sprintf("nc -v localhost %s", hostPort),
			"simulated_response": map[string]interface{}{
				"logs": []string{
					"Connection accepted from 127.0.0.1:49210",
					handshakeMsg,
					"Awaiting data payload...",
					"Received payload: 'GET / HTTP/1.1\\r\\nHost: localhost...'",
					"Connection closed by server",
				},
			},
		})
	}
}

func handleGetLibrary(w http.ResponseWriter, r *http.Request) {
	catalog := loadStubsLibrary()
	
	activeStubsPath := filepath.Join(getUniverseDir(WORKSPACE_ID), "active_stubs.json")
	activeStubsList := []string{}
	if body, err := os.ReadFile(activeStubsPath); err == nil {
		_ = json.Unmarshal(body, &activeStubsList)
	} else if os.IsNotExist(err) && WORKSPACE_ID != "" {
		activeStubsList = []string{}
		bodyBytes, _ := json.MarshalIndent(activeStubsList, "", "  ")
		_ = os.WriteFile(activeStubsPath, bodyBytes, 0644)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"catalog":      catalog,
		"active_stubs": activeStubsList,
	})
}

func handleToggleAllStubs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	activeStubsPath := filepath.Join(getUniverseDir(WORKSPACE_ID), "active_stubs.json")
	activeStubsList := []string{}

	if body.Active {
		catalog := loadStubsLibrary()
		for k := range catalog {
			activeStubsList = append(activeStubsList, k)
		}
	} else {
		activeStubsList = []string{}
	}

	bodyBytes, _ := json.MarshalIndent(activeStubsList, "", "  ")
	_ = os.WriteFile(activeStubsPath, bodyBytes, 0644)

	// Refresh internal domains mappings
	if _, err := os.Stat(COMPOSE_PATH); err == nil {
		bodyText, _ := os.ReadFile(COMPOSE_PATH)
		_ = initializeStateFromCompose(string(bodyText))
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"active_stubs": activeStubsList,
	})
}

func handleToggleActiveStub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Protocol string `json:"protocol"`
		Active   bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	protoKey := strings.ToLower(strings.TrimSpace(body.Protocol))
	if protoKey == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Protocol key is required"})
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	activeStubsPath := filepath.Join(getUniverseDir(WORKSPACE_ID), "active_stubs.json")
	activeStubsList := []string{}
	if data, err := os.ReadFile(activeStubsPath); err == nil {
		_ = json.Unmarshal(data, &activeStubsList)
	} else if os.IsNotExist(err) && WORKSPACE_ID != "" {
		activeStubsList = []string{}
	}

	exists := false
	for _, p := range activeStubsList {
		if p == protoKey {
			exists = true
			break
		}
	}

	if body.Active && !exists {
		activeStubsList = append(activeStubsList, protoKey)
	} else if !body.Active && exists {
		newList := []string{}
		for _, p := range activeStubsList {
			if p != protoKey {
				newList = append(newList, p)
			}
		}
		activeStubsList = newList
	}

	bodyBytes, _ := json.MarshalIndent(activeStubsList, "", "  ")
	_ = os.WriteFile(activeStubsPath, bodyBytes, 0644)

	// Refresh internal domains mappings
	if _, err := os.Stat(COMPOSE_PATH); err == nil {
		body, _ := os.ReadFile(COMPOSE_PATH)
		_ = initializeStateFromCompose(string(body))
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"active_stubs": activeStubsList,
	})
}

func handleAddCustomProtocol(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Protocol    string `json:"protocol"`
		DefaultPort int    `json:"default_port"`
		Description string `json:"description"`
		Code        string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	protoKey := strings.ToLower(strings.TrimSpace(body.Protocol))
	if protoKey == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Protocol key is required"})
		return
	}

	if _, ok := stubsCatalog[protoKey]; ok {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Cannot overwrite built-in protocols"})
		return
	}

	catalog := loadStubsLibrary()
	newStub := map[string]interface{}{
		"name":         body.Name,
		"protocol":     protoKey,
		"default_port": body.DefaultPort,
		"description":  body.Description,
		"code":         body.Code,
	}
	catalog[protoKey] = newStub
	saveCustomStubs(catalog)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"protocol": newStub,
	})
}

func handleExportMockCompose(w http.ResponseWriter, r *http.Request) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	res := exportMockCompose()
	if _, ok := res["error"]; ok {
		jsonResponse(w, http.StatusBadRequest, res)
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

type PortConflict struct {
	Port              int    `json:"port"`
	ConflictType      string `json:"conflict_type"` // "universe" or "external"
	CompetingUniverse string `json:"competing_universe_id,omitempty"`
	CompetingName     string `json:"competing_universe_name,omitempty"`
}

func checkPortsConflicts(hostPorts []int) ([]PortConflict, error) {
	var conflicts []PortConflict

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	gcfg, _ := readGlobalConfig()

	for _, port := range hostPorts {
		competingID := ""
		dockerBound := false
		
		for _, c := range containers {
			if c.State != "running" {
				continue
			}
			cid := c.Labels["com.mockdock.universe"]
			if cid == "" {
				cid = c.Labels["com.docker.compose.project"]
			}
			if cid != "" && cid == WORKSPACE_ID {
				continue
			}
			
			bound := false
			for _, cp := range c.Ports {
				if cp.PublicPort == uint16(port) {
					bound = true
					break
				}
			}
			if bound {
				dockerBound = true
				competingID = c.Labels["com.mockdock.universe"]
				if competingID == "" {
					competingID = c.Labels["com.docker.compose.project"]
				}
				break
			}
		}

		if dockerBound {
			name := competingID
			isUniverse := false
			if gcfg != nil && competingID != "" {
				if ws, ok := gcfg.Workspaces[competingID]; ok {
					name = ws.Name
					isUniverse = true
				}
			}
			
			if isUniverse {
				conflicts = append(conflicts, PortConflict{
					Port:              port,
					ConflictType:      "universe",
					CompetingUniverse: competingID,
					CompetingName:     name,
				})
			} else {
				conflicts = append(conflicts, PortConflict{
					Port:         port,
					ConflictType: "external",
				})
			}
			continue
		}

		if isPortOccupied(strconv.Itoa(port)) {
			conflicts = append(conflicts, PortConflict{
				Port:         port,
				ConflictType: "external",
			})
		}
	}

	return conflicts, nil
}

func handleWorkspaceSleep(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("id")
	if wsID == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Missing workspace id parameter"})
		return
	}

	gcfg, err := readGlobalConfig()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": "Failed to read global config"})
		return
	}

	ws, ok := gcfg.Workspaces[wsID]
	if !ok {
		jsonResponse(w, http.StatusNotFound, map[string]string{"detail": "Workspace not found"})
		return
	}

	prevCompose := filepath.Join(getUniverseDir(ws.ID), "source-compose.yaml")
	prevMocked := filepath.Join(getUniverseDir(ws.ID), "mocked-compose.yaml")
	if _, errC := os.Stat(prevCompose); errC == nil {
		args := []string{"compose", "--project-directory", ws.Path, "-f", prevCompose}
		if _, errM := os.Stat(prevMocked); errM == nil {
			args = append(args, "-f", prevMocked)
		}
		args = append(args, "down")
		cmd := exec.Command("docker", args...)
		cmd.Dir = ws.Path
		output, errRun := cmd.CombinedOutput()
		if errRun != nil {
			log.Printf("[Sleep] Failed to put workspace %s to sleep: %v. Output: %s", ws.ID, errRun, string(output))
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func handleSaveMock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		ServiceName string `json:"service_name"`
		Method      string `json:"method"`
		Path        string `json:"path"`
		JSONBody    string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Invalid JSON request"})
		return
	}

	workspaceID := strings.TrimSpace(body.WorkspaceID)
	serviceName := strings.TrimSpace(body.ServiceName)
	method := strings.ToUpper(strings.TrimSpace(body.Method))
	path := strings.TrimSpace(body.Path)
	
	if workspaceID == "" || serviceName == "" || method == "" || path == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Missing required fields (workspace_id, service_name, method, path)"})
		return
	}

	// Check if body is valid JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(body.JSONBody), &parsed); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Body must be valid JSON"})
		return
	}

	cleanPath := strings.Trim(path, "/")
	cleanPath = strings.ReplaceAll(cleanPath, "/", "_")
	if cleanPath == "" {
		cleanPath = "root"
	}

	// Create data directory for this universe and service
	universeDir := getUniverseDir(workspaceID)
	dataDir := filepath.Join(universeDir, "data", serviceName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": fmt.Sprintf("Failed to create data directory: %v", err)})
		return
	}

	filename := fmt.Sprintf("%s_%s.json", method, cleanPath)
	destFile := filepath.Join(dataDir, filename)
	
	// Indent JSON to keep files readable on disk
	indentedBytes, _ := json.MarshalIndent(parsed, "", "  ")
	if err := os.WriteFile(destFile, indentedBytes, 0644); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"detail": fmt.Sprintf("Failed to write mock file: %v", err)})
		return
	}

	// Update registry.json mapping
	registryFile := filepath.Join(dataDir, "registry.json")
	registry := make(map[string]string)
	if regBytes, err := os.ReadFile(registryFile); err == nil {
		_ = json.Unmarshal(regBytes, &registry)
	}
	registry[filename] = path
	if regBytes, err := json.MarshalIndent(registry, "", "  "); err == nil {
		_ = os.WriteFile(registryFile, regBytes, 0644)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true, "file": filename})
}

func handleListMocks(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	serviceName := r.URL.Query().Get("service_name")

	if workspaceID == "" || serviceName == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "Missing workspace_id or service_name query params"})
		return
	}

	universeDir := getUniverseDir(workspaceID)
	dataDir := filepath.Join(universeDir, "data", serviceName)

	registryFile := filepath.Join(dataDir, "registry.json")
	registry := make(map[string]string)
	if regBytes, err := os.ReadFile(registryFile); err == nil {
		_ = json.Unmarshal(regBytes, &registry)
	}

	files, err := os.ReadDir(dataDir)
	if err != nil {
		jsonResponse(w, http.StatusOK, []interface{}{})
		return
	}

	type MockItem struct {
		Filename string `json:"filename"`
		Method   string `json:"method"`
		Path     string `json:"path"`
		Body     string `json:"body"`
	}

	var results []MockItem
	for _, f := range files {
		if f.IsDir() || f.Name() == "registry.json" || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dataDir, f.Name())
		contentBytes, errRead := os.ReadFile(filePath)
		if errRead != nil {
			continue
		}

		filename := f.Name()
		method := ""
		pathVal := ""

		if p, ok := registry[filename]; ok {
			pathVal = p
			firstUnderscore := strings.Index(filename, "_")
			if firstUnderscore > 0 {
				method = filename[:firstUnderscore]
			}
		}

		if method == "" || pathVal == "" {
			firstUnderscore := strings.Index(filename, "_")
			if firstUnderscore > 0 {
				method = filename[:firstUnderscore]
				cleanPath := filename[firstUnderscore+1 : len(filename)-5]
				if cleanPath == "root" {
					pathVal = "/"
				} else {
					pathVal = "/" + strings.ReplaceAll(cleanPath, "_", "/")
				}
			}
		}

		results = append(results, MockItem{
			Filename: filename,
			Method:   method,
			Path:     pathVal,
			Body:     string(contentBytes),
		})
	}

	jsonResponse(w, http.StatusOK, results)
}

func handleComposeUp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Build bool `json:"build"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Check for port conflicts before starting the stack
	ports, _ := getComposeHostPorts()
	conflicts, errCheck := checkPortsConflicts(ports)
	if errCheck == nil && len(conflicts) > 0 {
		hasUniverseConflict := false
		for _, conf := range conflicts {
			if conf.ConflictType == "universe" {
				hasUniverseConflict = true
				break
			}
		}
		if hasUniverseConflict {
			jsonResponse(w, http.StatusConflict, map[string]interface{}{
				"success":   false,
				"conflicts": conflicts,
			})
			return
		}
	}

	stateMutex.Lock()
	// Clean and populate port remappings
	portRemappings = make(map[int]int)
	state.PortRemappings = []PortRemapInfo{}

	for _, conf := range conflicts {
		if conf.ConflictType == "external" {
			freePort := findFreePort(conf.Port + 1)
			if freePort > 0 {
				portRemappings[conf.Port] = freePort
				state.PortRemappings = append(state.PortRemappings, PortRemapInfo{
					OriginalPort: conf.Port,
					RemappedPort: freePort,
				})
				log.Printf("[Port Manager] Port %d is occupied by an external host process. Re-mapped to %d.", conf.Port, freePort)
			}
		}
	}

	// Sync active profile on disk before up
	_ = saveProfileInternalLocked(ACTIVE_PROFILE)
	// Run export first to apply labels and remapped ports
	exportMockCompose()
	stateMutex.Unlock()

	stdout, stderr, err := runComposeUp(body.Build)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"stdout":  stdout,
			"stderr":  stderr,
			"detail":  formatComposeError(stderr, err),
		})
		return
	}

	// Apply tc rules for all running REAL services that have chaos settings!
	go func() {
		// Wait a brief moment for containers to boot up
		time.Sleep(3 * time.Second)
		stateMutex.Lock()
		defer stateMutex.Unlock()
		for name, svc := range state.Services {
			if !svc.Mocked && (svc.ChaosConfig.LatencyMS > 0 || svc.ChaosConfig.PacketLossPct > 0) {
				containerID, err := getContainerIDByService(name)
				if err == nil && containerID != "" {
					_ = applyTrafficControl(containerID, svc.ChaosConfig.LatencyMS, svc.ChaosConfig.PacketLossPct)
				}
			}
		}
	}()

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"stdout":  stdout,
		"stderr":  stderr,
	})
}

func handleComposeDown(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := runComposeDown()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"stdout":  stdout,
			"stderr":  stderr,
			"detail":  fmt.Sprintf("Docker Compose down failed: %v", err),
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"stdout":  stdout,
		"stderr":  stderr,
	})
}

func handleComposeStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(MOCKED_COMPOSE_PATH); os.IsNotExist(err) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"running":    false,
			"containers": []interface{}{},
		})
		return
	}

	containers, err := getDockerContainers()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{
			"running":    false,
			"containers": []interface{}{},
			"error":      err.Error(),
		})
		return
	}

	// Filter down to match containers of the project if possible
	running := false
	for _, val := range containers {
		if cMap, ok := val.(map[string]interface{}); ok {
			stateStr := strings.ToLower(fmt.Sprintf("%v", cMap["State"]))
			statusStr := strings.ToLower(fmt.Sprintf("%v", cMap["Status"]))
			
			// We check if the container is running and has a Service tag matching state
			svcName := fmt.Sprintf("%v", cMap["Service"])
			if svcName != "" {
				if _, ok := state.Services[svcName]; ok {
					if strings.Contains(stateStr, "running") || strings.Contains(statusStr, "up") {
						running = true
					}
				}
			}
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"running":    running,
		"containers": containers,
	})
}

func sendCliRequest(method string, path string, body []byte) (*http.Response, error) {
	gcfg, _ := readGlobalConfig()
	token := ""
	if gcfg != nil {
		token = gcfg.AuthToken
	}

	req, err := http.NewRequest(method, fmt.Sprintf("%s%s", DAEMON_URL, path), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// CLI Mode Functions
func runCliClient(cmd string, buildFlag bool) {
	switch cmd {
	case "up":
		// Sync docker-compose.yml first
		if _, err := os.Stat("docker-compose.yml"); err == nil {
			body, _ := os.ReadFile("docker-compose.yml")
			log.Println("🔄 Syncing local docker-compose.yml configuration with daemon...")
			
			reqBody, _ := json.Marshal(map[string]string{"yaml_content": string(body)})
			resp, err := sendCliRequest("POST", "/api/compose/update", reqBody)
			if err != nil {
				log.Printf("⚠️ Warning: Failed to sync config: %v. Proceeding with daemon's cache...", err)
			} else {
				resp.Body.Close()
			}
		}

		log.Printf("🚀 Starting mock compose stack (build=%t)...", buildFlag)
		reqBody, _ := json.Marshal(map[string]bool{"build": buildFlag})
		resp, err := sendCliRequest("POST", "/api/compose/up", reqBody)
		if err != nil {
			log.Fatalf("❌ Connection error: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		if resp.StatusCode == http.StatusOK {
			log.Println("✅ Stack started successfully!")
			if stdout, ok := result["stdout"].(string); ok && stdout != "" {
				fmt.Println(stdout)
			}
		} else {
			log.Fatalf("❌ Failed to start stack: %v", result["detail"])
		}

	case "down":
		log.Println("🛑 Stopping mock compose stack...")
		resp, err := sendCliRequest("POST", "/api/compose/down", nil)
		if err != nil {
			log.Fatalf("❌ Connection error: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		if resp.StatusCode == http.StatusOK {
			log.Println("✅ Stack stopped and cleaned up successfully!")
			if stdout, ok := result["stdout"].(string); ok && stdout != "" {
				fmt.Println(stdout)
			}
		} else {
			log.Fatalf("❌ Failed to stop stack: %v", result["detail"])
		}

	case "status":
		resp, err := sendCliRequest("GET", "/api/compose", nil)
		if err != nil {
			log.Fatalf("❌ Error: Cannot connect to the MockDock daemon. Is it running?")
		}
		defer resp.Body.Close()
		var composeState map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&composeState)

		respStatus, err := sendCliRequest("GET", "/api/compose/status", nil)
		if err != nil {
			log.Fatalf("❌ Error: Cannot fetch stack status: %v", err)
		}
		defer respStatus.Body.Close()
		var statusState map[string]interface{}
		_ = json.NewDecoder(respStatus.Body).Decode(&statusState)

		project := composeState["project_label"]
		running := statusState["running"].(bool)
		runningText := "Stack Stopped"
		icon := "⚪"
		if running {
			runningText = "Stack Running"
			icon = "🟢"
		}

		fmt.Printf("Workspace Project : %v\n", project)
		fmt.Printf("Runtime Status    : %s %s\n", icon, runningText)
		fmt.Println(strings.Repeat("-", 75))
		fmt.Printf("%-12s | %-6s | %-20s | %-10s | %s\n", "Service", "Mode", "Container Name", "Status", "Port Mappings")
		fmt.Println(strings.Repeat("-", 75))

		// Map runtime states
		runtimeMap := map[string]interface{}{}
		if containers, ok := statusState["containers"].([]interface{}); ok {
			for _, c := range containers {
				if cMap, ok := c.(map[string]interface{}); ok {
					svc := fmt.Sprintf("%v", cMap["Service"])
					if svc != "" {
						runtimeMap[svc] = cMap
					}
				}
			}
		}

		if services, ok := composeState["services"].([]interface{}); ok {
			for _, s := range services {
				sMap, ok := s.(map[string]interface{})
				if !ok {
					continue
				}
				name := fmt.Sprintf("%v", sMap["name"])
				mocked := sMap["mocked"].(bool)
				mode := "REAL"
				if mocked {
					mode = "GHOST"
				}

				cName := "N/A"
				cState := "stopped"
				if cMap, ok := runtimeMap[name].(map[string]interface{}); ok {
					cName = fmt.Sprintf("%v", cMap["Name"])
					cState = fmt.Sprintf("%v", cMap["State"])
				}

				portsList := []string{}
				if ports, ok := sMap["ports"].([]interface{}); ok {
					for _, p := range ports {
						portsList = append(portsList, fmt.Sprintf("%v", p))
					}
				}
				portsText := "internal"
				if len(portsList) > 0 {
					portsText = strings.Join(portsList, ", ")
				}

				fmt.Printf("%-12s | %-6s | %-20s | %-10s | %s\n", name, mode, cName, cState, portsText)
			}
		}
		fmt.Println(strings.Repeat("-", 75))
	}
}

const DAEMON_URL = "http://localhost:11800"

func cleanupAllChaos() {
	log.Println("🧹 Cleaning up all container chaos rules before exit...")
	stateMutex.Lock()
	defer stateMutex.Unlock()

	for name, svc := range state.Services {
		if !svc.Mocked && (svc.ChaosConfig.LatencyMS > 0 || svc.ChaosConfig.PacketLossPct > 0) {
			containerID, err := getContainerIDByService(name)
			if err == nil && containerID != "" {
				log.Printf("[Chaos] Clearing chaos for %s (%s)...", name, containerID)
				_ = clearTrafficControl(containerID)
			}
		}
	}
}

func initWorkspaceFromCli(projectName, composePath string) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("❌ Failed to get current working directory: %v", err)
	}

	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		log.Fatalf("❌ Project name cannot be empty")
	}

	// 1. Create the mockdock directory in their project
	repoMockdockDir := filepath.Join(cwd, "mockdock")
	err = os.MkdirAll(repoMockdockDir, 0755)
	if err != nil {
		log.Fatalf("❌ Failed to create mockdock directory: %v", err)
	}

	// 2. Create the .mockdock file with the project name
	workspaceID := strings.ToLower(projectName)
	workspaceID = strings.ReplaceAll(workspaceID, " ", "-")
	markerPath := filepath.Join(repoMockdockDir, ".mockdock")
	err = os.WriteFile(markerPath, []byte(fmt.Sprintf("id: %s\n", workspaceID)), 0644)
	if err != nil {
		log.Fatalf("❌ Failed to create .mockdock marker file: %v", err)
	}

	// Set paths dynamically so validator resolves relative paths (.env) relative to CWD
	WORKSPACE_DIR = cwd
	WORKSPACE_ID = workspaceID

	// 3. Read the compose file specified
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		log.Fatalf("❌ Failed to read compose file at %s: %v", composePath, err)
	}

	// Validate compose file structure
	validationRes := initializeStateFromCompose(string(composeData))
	if errMsg, ok := validationRes["error"]; ok {
		log.Fatalf("❌ Failed to initialize universe: Compose file validation failed:\n   %v", errMsg)
	}

	// Check if any ports in the compose file are already occupied on the host
	for name, svc := range state.Services {
		for _, portMapping := range svc.Ports {
			hostPort := getHostPort(portMapping)
			if hostPort != "" {
				conn, errDial := net.DialTimeout("tcp", "127.0.0.1:"+hostPort, 100*time.Millisecond)
				if errDial == nil {
					conn.Close()
					fmt.Printf("⚠️  Warning: Port %s (required by service '%s') is already in use on your host machine. You may need to stop that process or configure this service in Ghost mode.\n", hostPort, name)
				}
			}
		}
	}

	// Create project/mockdock/sources directory
	repoSourcesDir := filepath.Join(repoMockdockDir, "sources")
	err = os.MkdirAll(repoSourcesDir, 0755)
	if err != nil {
		log.Fatalf("❌ Failed to create sources directory: %v", err)
	}

	// Save the compose file in mockdock/sources
	composeFilename := filepath.Base(composePath)
	destPath := filepath.Join(repoSourcesDir, composeFilename)
	err = os.WriteFile(destPath, composeData, 0644)
	if err != nil {
		log.Fatalf("❌ Failed to write compose source file: %v", err)
	}

	// 4. Register the universe workspace globally
	gcfg, err := readGlobalConfig()
	if err != nil {
		log.Fatalf("❌ Failed to read global config: %v", err)
	}
	gcfg.Workspaces[workspaceID] = WorkspaceMetadata{
		ID:   workspaceID,
		Path: cwd,
		Name: projectName,
	}
	err = writeGlobalConfig(gcfg)
	if err != nil {
		log.Fatalf("❌ Failed to save global config: %v", err)
	}

	// 5. Cache the main blueprint inside ~/.mockdock/universes/[workspace-id]/source-compose.yaml
	universeDir := getUniverseDir(workspaceID)
	_ = os.MkdirAll(universeDir, 0755)
	sourceCachePath := filepath.Join(universeDir, "source-compose.yaml")
	_ = os.WriteFile(sourceCachePath, sanitizeYamlNameField(composeData), 0644)

	// Save active sources list metadata inside global config profile / cache
	sourcesPath := filepath.Join(universeDir, "sources.json")
	_ = os.MkdirAll(filepath.Join(universeDir, "sources"), 0755)
	
	// Write dynamic sources JSON checklist configuration
	sourcesData := struct {
		ProjectLabel string       `json:"project_label"`
		Sources      []SourceItem `json:"sources"`
	}{
		ProjectLabel: projectName,
		Sources: []SourceItem{
			{
				Type:      "path",
				Value:     "mockdock/sources/" + composeFilename,
				Enabled:   true,
				IsDefault: false,
			},
		},
	}
	sourcesBytes, _ := json.MarshalIndent(sourcesData, "", "  ")
	_ = os.WriteFile(sourcesPath, sourcesBytes, 0644)

	fmt.Printf("🚀 Successfully initialized universe '%s'!\n", projectName)
	fmt.Printf("  - Folder created: ./mockdock/\n")
	fmt.Printf("  - Marker created: ./mockdock/.mockdock (id: %s)\n", workspaceID)
	fmt.Printf("  - Source saved: ./mockdock/sources/%s\n", composeFilename)
	fmt.Printf("  - Registered globally at: %s\n", cwd)
	fmt.Println("\nTo activate this universe:")
	fmt.Printf("   👉 Web Dashboard: Go to the 'Profiles' tab, select '%s' and click 'Activate'.\n", projectName)
	fmt.Printf("   👉 Command Line: Run: mockdock activate %s\n", workspaceID)
}

func formatComposeError(stderr string, err error) string {
	if strings.Contains(stderr, "unable to prepare context") {
		return "Build context directory not found. Please create the required build folders inside your repository or put the service in Ghost mode to mock it."
	}
	if strings.Contains(stderr, "port is already allocated") || strings.Contains(stderr, "Bind for 0.0.0.0") || strings.Contains(stderr, "address already in use") {
		if strings.Contains(stderr, "5000") {
			return "Port 5000 conflict: This port is commonly occupied by the macOS AirPlay Receiver process. Please disable AirPlay Receiver in macOS System Settings (General -> AirPlay & Handoff) or put the conflicting service in Ghost mode."
		}
		return "Port conflict: One of the container ports is already allocated by another process (like the MockDock daemon on port 11800)."
	}
	return fmt.Sprintf("Docker Compose failed: %v", err)
}

var DAEMON_AUTH_TOKEN string

func main() {
	// Parse CLI flags and commands
	initPaths()
	initLicensing()
	initCatalog()

	gcfg, err := readGlobalConfig()
	if err == nil && gcfg != nil {
		DAEMON_AUTH_TOKEN = gcfg.AuthToken
	}

	if len(os.Args) >= 2 {
		cmd := strings.ToLower(os.Args[1])
		switch cmd {
		case "stub-config", "stub-wizard":
			fs := flag.NewFlagSet("stub-config", flag.ExitOnError)
			service := fs.String("service", "", "Service name to configure")
			protocol := fs.String("protocol", "http", "Protocol stub engine (http, postgres, mysql, redis, mongodb, rabbitmq, tcp, none)")
			status := fs.Int("status", 200, "HTTP Status Code")
			body := fs.String("body", "", "JSON default global response body")
			script := fs.String("script", "", "JS scripting code or filepath")
			crud := fs.Bool("crud", false, "Enable stateful CRUD collections")
			sqlite := fs.Bool("sqlite", false, "Enable SQL-to-SQLite DB mapping")
			latency := fs.Int("latency", 0, "Latency in milliseconds")
			loss := fs.Int("loss", 0, "Packet loss percentage")
			mode := fs.String("mode", "ghost", "Service mode (ghost or real)")
			_ = fs.Parse(os.Args[2:])

			if *service == "" {
				log.Fatalf("❌ Missing required flag: --service")
			}

			// If script is a file path, load its content
			scriptContent := *script
			if *script != "" {
				if _, err := os.Stat(*script); err == nil {
					contentBytes, errRead := os.ReadFile(*script)
					if errRead == nil {
						scriptContent = string(contentBytes)
					}
				}
			}

			// 1. Toggle Mode
			isMocked := strings.ToLower(*mode) == "ghost"
			togglePayload, _ := json.Marshal(map[string]interface{}{
				"service_name": *service,
				"mocked":       isMocked,
			})
			respToggle, err := sendCliRequest("POST", "/api/services/toggle", togglePayload)
			if err != nil {
				log.Fatalf("❌ Failed to toggle service mode: %v", err)
			}
			respToggle.Body.Close()

			// 2. Configure Stub Settings
			stubPayload, _ := json.Marshal(map[string]interface{}{
				"service_name":      *service,
				"protocol":          *protocol,
				"http_status":       *status,
				"response_body":     *body,
				"tcp_log_handshake": true,
				"script":            scriptContent,
				"http_crud":         *crud,
				"sqlite_enabled":    *sqlite,
			})
			respStub, err := sendCliRequest("POST", "/api/services/stub", stubPayload)
			if err != nil {
				log.Fatalf("❌ Failed to configure stub: %v", err)
			}
			respStub.Body.Close()

			// 3. Configure Chaos (Latency / Loss)
			chaosPayload, _ := json.Marshal(map[string]interface{}{
				"service_name":    *service,
				"latency_ms":      *latency,
				"packet_loss_pct": *loss,
				"disconnected":    false,
			})
			respChaos, err := sendCliRequest("POST", "/api/chaos", chaosPayload)
			if err != nil {
				log.Fatalf("❌ Failed to configure chaos settings: %v", err)
			}
			respChaos.Body.Close()

			log.Printf("✅ Successfully configured stub wizard for service '%s'!", *service)
			log.Printf("   Mode     : %s", strings.ToUpper(*mode))
			log.Printf("   Engine   : %s", strings.ToUpper(*protocol))
			if *protocol == "http" {
				log.Printf("   Status   : %d", *status)
				log.Printf("   CRUD     : %t", *crud)
			}
			if *latency > 0 {
				log.Printf("   Latency  : %dms", *latency)
			}
			if *loss > 0 {
				log.Printf("   Loss     : %d%%", *loss)
			}
			return
		case "up":
			buildFlag := false
			for _, arg := range os.Args[2:] {
				if arg == "--build" {
					buildFlag = true
				}
			}
			runCliClient("up", buildFlag)
			return
		case "down":
			runCliClient("down", false)
			return
		case "status":
			runCliClient("status", false)
			return
		case "stub":
			fs := flag.NewFlagSet("stub", flag.ExitOnError)
			proto := fs.String("protocol", "tcp", "Protocol to mock")
			port := fs.Int("port", 8080, "Port to listen on")
			httpStatus := fs.Int("http-status", 200, "HTTP Status Code for HTTP mocks")
			respBody := fs.String("response-body", `{"status":"ok"}`, "HTTP Response Body")
			logH := fs.Bool("log-handshake", true, "Log handshake events to stdout")
			latency := fs.Int("latency", 0, "Latency delay in milliseconds")
			loss := fs.Int("loss", 0, "Packet loss rate in percentage")
			script := fs.String("script", "", "JS Script to execute for HTTP mocks")
			httpCRUD := fs.Bool("http-crud", false, "Enable stateful CRUD logic")
			sqliteEnabled := fs.Bool("sqlite-enabled", false, "Enable SQL-to-SQLite translation for database stubs")
			llmProvider := fs.String("llm-provider", "", "LLM Provider")
			llmModel := fs.String("llm-model", "", "LLM Model name")
			llmStream := fs.Bool("llm-stream", false, "Enable SSE streaming")
			llmRateLimit := fs.Bool("llm-rate-limit", false, "Enable Rate Limit simulation")
			llmTokensLimit := fs.Int("llm-tokens-limit", 0, "Tokens per minute limit")
			llmReqsLimit := fs.Int("llm-reqs-limit", 0, "Requests per minute limit")
			llmTtft := fs.Int("llm-ttft", 0, "TTFT delay in ms")
			llmTokenDelay := fs.Int("llm-token-delay", 0, "Inter-token delay in ms")
			chaosJitter := fs.Int("chaos-jitter", 0, "Latency jitter in ms")
			chaosBandwidth := fs.Int("chaos-bandwidth", 0, "Bandwidth limit in kbps")
			chaosDnsStatus := fs.String("chaos-dns-status", "normal", "DNS Status")
			chaosDnsDelay := fs.Int("chaos-dns-delay", 0, "DNS lookup delay in ms")
			chaosTruncated := fs.Bool("chaos-truncated", false, "Truncated response")
			chaosCpu := fs.Int("chaos-cpu", 0, "CPU Spike %")
			chaosMem := fs.Int("chaos-mem", 0, "Memory Spike MB")
			_ = fs.Parse(os.Args[2:])
			
			runStub(*proto, *port, *httpStatus, *respBody, *logH, *latency, *loss, *script, *httpCRUD, *sqliteEnabled,
				*llmProvider, *llmModel, *llmStream, *llmRateLimit, *llmTokensLimit, *llmReqsLimit, *llmTtft, *llmTokenDelay,
				*chaosJitter, *chaosBandwidth, *chaosDnsStatus, *chaosDnsDelay, *chaosTruncated, *chaosCpu, *chaosMem)
			return
		case "mcp":
			startMcpServer()
			return
		case "test-labs-install":
			installTestLabs()
			return
		case "activate":
			if len(os.Args) < 3 {
				log.Fatalf("❌ Missing project name. Usage: mockdock activate [project name]")
			}
			projectName := strings.TrimSpace(os.Args[2])
			gcfg, err := readGlobalConfig()
			if err != nil {
				log.Fatalf("❌ Failed to read global config: %v", err)
			}
			
			// Find match by ID or Name
			var matchedID string
			for id, ws := range gcfg.Workspaces {
				if strings.EqualFold(id, projectName) || strings.EqualFold(ws.Name, projectName) {
					matchedID = id
					break
				}
			}
			
			if matchedID == "" {
				log.Fatalf("❌ Universe workspace '%s' not found. Register it first using: mockdock %s [path-to-compose]", projectName, projectName)
			}

			if !universeHasSource(matchedID, gcfg.Workspaces[matchedID].Path) {
				log.Fatalf("❌ Cannot activate universe '%s': no configuration sources found. Please place a compose file in your project or mockdock/sources/ directory first.", gcfg.Workspaces[matchedID].Name)
			}
			
			gcfg.ActiveWorkspace = matchedID
			err = writeGlobalConfig(gcfg)
			if err != nil {
				log.Fatalf("❌ Failed to update active workspace: %v", err)
			}
			
			fmt.Printf("🚀 Successfully activated universe '%s'!\n", gcfg.Workspaces[matchedID].Name)
			return
		case "chaos":
			if !IS_RAD_ACTIVE {
				log.Fatalf("❌ MockDockRAD license key is required to use premium CLI commands. Activate via Settings or run 'mockdock license activate [key]'.")
			}
			
			fs := flag.NewFlagSet("chaos", flag.ExitOnError)
			svcOpt := fs.String("service", "", "Compose service name to inject chaos")
			latencyOpt := fs.String("latency", "0ms", "Latency delay (e.g., 500ms)")
			lossOpt := fs.String("loss", "0%", "Packet loss percentage (e.g., 10%)")
			durOpt := fs.String("duration", "30s", "Duration to apply chaos (e.g., 30s)")
			
			startIdx := 2
			if len(os.Args) >= 3 && strings.ToLower(os.Args[2]) == "run" {
				startIdx = 3
			}
			_ = fs.Parse(os.Args[startIdx:])
			
			if *svcOpt == "" {
				log.Fatalf("❌ Missing required parameter --service")
			}
			
			latencyVal := parseLatency(*latencyOpt)
			lossVal := parseLoss(*lossOpt)
			durVal := parseDuration(*durOpt)
			
			initializeCLIState()
			
			containerID, err := getContainerIDByService(*svcOpt)
			if err != nil {
				log.Fatalf("❌ Failed to find container for service %s: %v", *svcOpt, err)
			}
			
			fmt.Printf("🔥 Injecting network chaos into service '%s' (container: %s) for %s...\n", *svcOpt, containerID[:12], *durOpt)
			fmt.Printf("   Config: latency=%dms, packet_loss=%d%%\n", latencyVal, lossVal)
			
			err = applyTrafficControl(containerID, latencyVal, lossVal)
			if err != nil {
				log.Fatalf("❌ Chaos injection failed: %v", err)
			}
			
			fmt.Printf("⏱️ Running for %s... (Press Ctrl+C to cancel and restore normal traffic early)\n", *durOpt)
			
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-c
				fmt.Println("\n🧹 Restoring normal traffic control namespace early...")
				_ = clearTrafficControl(containerID)
				os.Exit(0)
			}()
			
			time.Sleep(durVal)
			
			fmt.Println("🧹 Restoring normal traffic control namespace...")
			_ = clearTrafficControl(containerID)
			fmt.Println("✅ Chaos run completed.")
			return
		case "audit":
			if !IS_RAD_ACTIVE {
				log.Fatalf("❌ MockDockRAD license key is required to use premium CLI commands. Activate via Settings or run 'mockdock license activate [key]'.")
			}
			fs := flag.NewFlagSet("audit", flag.ExitOnError)
			strict := fs.Bool("strict", false, "Exit with non-zero code on warning findings")
			_ = fs.Parse(os.Args[2:])
			
			initializeCLIState()
			runAudit()
			
			fmt.Printf("🔍 Running security audit for active universe: %s\n", state.ProjectLabel)
			fmt.Printf("📊 Security Score: %d/100\n", state.AuditResults.Score)
			fmt.Printf("⚠️ Warnings Found: %d\n\n", len(state.AuditResults.Warnings))
			
			hasHighOrMedium := false
			for _, w := range state.AuditResults.Warnings {
				fmt.Printf("[%s] Service: %s\n", w.Severity, w.Service)
				fmt.Printf("  Message: %s\n\n", w.Message)
				if w.Severity == "High" || w.Severity == "Medium" {
					hasHighOrMedium = true
				}
			}
			
			if *strict && len(state.AuditResults.Warnings) > 0 && hasHighOrMedium {
				fmt.Println("❌ Audit failed strict checks (High/Medium issues found). Exiting.")
				os.Exit(1)
			}
			fmt.Println("✅ Audit completed successfully.")
			return
		case "logs":
			if !IS_RAD_ACTIVE {
				log.Fatalf("❌ MockDockRAD license key is required to use premium CLI commands. Activate via Settings or run 'mockdock license activate [key]'.")
			}
			fs := flag.NewFlagSet("logs", flag.ExitOnError)
			format := fs.String("format", "junit", "Export logs format (junit)")
			
			startIdx := 2
			if len(os.Args) >= 3 && strings.ToLower(os.Args[2]) == "export" {
				startIdx = 3
			}
			_ = fs.Parse(os.Args[startIdx:])
			
			if *format != "junit" {
				log.Fatalf("❌ Unsupported logs format: %s. Only 'junit' is supported.", *format)
			}
			
			initializeCLIState()
			
			fmt.Println(`<?xml version="1.0" encoding="UTF-8"?>`)
			fmt.Println(`<testsuites name="MockDock Handshakes Trace">`)
			
			mockCounts := 0
			for _, svc := range state.Services {
				if svc.Mocked {
					mockCounts++
				}
			}
			
			fmt.Printf("  <testsuite name=\"Service Stubs\" tests=\"%d\" failures=\"0\" errors=\"0\">\n", mockCounts)
			for name, svc := range state.Services {
				if svc.Mocked {
					fmt.Printf("    <testcase name=\"stub_%s_handshake\" classname=\"mockdock.%s\">\n", name, svc.StubConfig.Protocol)
					fmt.Printf("      <system-out>Interception established on port %d</system-out>\n", getDefaultPort(svc.StubConfig.Protocol))
					fmt.Println("    </testcase>")
				}
			}
			fmt.Println("  </testsuite>")
			fmt.Println("</testsuites>")
			return
		case "license":
			if len(os.Args) < 3 {
				log.Fatalf("❌ Usage: mockdock license [status|activate] [key]")
			}
			sub := strings.ToLower(os.Args[2])
			if sub == "status" {
				info := checkLicenseState()
				if info.Active {
					fmt.Printf("💎 MockDockRAD is ACTIVE\nType: %s\nKey: %s\n", info.LicenseType, info.LicenseKey)
				} else {
					fmt.Println("❌ MockDockRAD is UNLICENSED (Free MockDock mode)")
				}
			} else if sub == "activate" {
				if len(os.Args) < 4 {
					log.Fatalf("❌ Missing license key. Usage: mockdock license activate [key]")
				}
				key := os.Args[3]
				info, err := activateLicenseKey(key)
				if err != nil {
					log.Fatalf("❌ Activation failed: %v", err)
				}
				fmt.Printf("🚀 Activated MockDockRAD successfully!\nType: %s\nKey: %s\n", info.LicenseType, info.LicenseKey)
			} else {
				log.Fatalf("❌ Unknown license subcommand: %s", sub)
			}
			return
		case "init":
			cwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("❌ Failed to get current working directory: %v", err)
			}
			
			composeFile := ""
			candidates := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
			for _, c := range candidates {
				if _, err := os.Stat(filepath.Join(cwd, c)); err == nil {
					composeFile = c
					break
				}
			}
			
			if composeFile == "" {
				log.Fatalf("❌ Error: No docker-compose.yml, docker-compose.yaml, compose.yml, or compose.yaml found in the current directory.")
			}
			
			projectName := filepath.Base(cwd)
			projectName = strings.TrimSuffix(projectName, "-main")
			projectName = strings.TrimSuffix(projectName, "-master")
			projectName = strings.ToLower(projectName)
			
			fmt.Printf("Initializing MockDock in current directory...\n")
			fmt.Printf("Project Name (default: %s): ", projectName)
			var inputName string
			_, _ = fmt.Scanln(&inputName)
			inputName = strings.TrimSpace(inputName)
			if inputName != "" {
				projectName = inputName
			}
			
			initWorkspaceFromCli(projectName, filepath.Join(cwd, composeFile))
			return
		case "daemon":
			// Proceed to run daemon
		default:
			if len(os.Args) == 3 {
				initWorkspaceFromCli(os.Args[1], os.Args[2])
				return
			}
			log.Fatalf("❌ Unknown command: %s\nRun 'mockdock status/up/down/[project-name] [compose-path]' for usage info.", cmd)
		}
	}

	// Daemon mode: load sources from workspace
	loadSourcesFromDisk()

	// Spin up External Service Interceptor dynamic listeners
	go runExternalHttpServer()
	go runExternalHttpsServer()

	// Setup OS exit signal hook to cleanup tc rules
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cleanupAllChaos()
		os.Exit(0)
	}()

	// Serve Static files for UI
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to retrieve embedded static assets: %v", err)
	}

	// Set routing layer
	mux := http.NewServeMux()
	noCacheFileServer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.FileServer(http.FS(staticSub)).ServeHTTP(w, r)
	})
	mux.Handle("/", noCacheFileServer)
	
	// Read-only endpoints
	mux.HandleFunc("/api/compose", handleGetCompose)
	mux.HandleFunc("/api/workspaces", handleListWorkspaces)
	mux.HandleFunc("/api/workspace/profiles", handleListProfiles)
	mux.HandleFunc("/api/compose/status", handleComposeStatus)
	mux.HandleFunc("/api/library", handleGetLibrary)
	mux.HandleFunc("/api/license/status", handleLicenseStatus)
	mux.HandleFunc("/api/logs", handleGetLogs)
	mux.HandleFunc("/api/auth/token", handleGetAuthToken)

	// Mutating endpoints (Authorization protected)
	mux.Handle("/api/compose/update", requireAuth(http.HandlerFunc(handleUpdateCompose)))
	mux.Handle("/api/compose/sources", requireAuth(http.HandlerFunc(handleUpdateSources)))
	mux.Handle("/api/compose/sources/upload", requireAuth(http.HandlerFunc(handleUploadSourceFile)))
	mux.Handle("/api/universe/add", requireAuth(http.HandlerFunc(handleAddUniverse)))
	mux.Handle("/api/universe/remove", requireAuth(http.HandlerFunc(handleRemoveUniverse)))
	mux.Handle("/api/workspace/select", requireAuth(http.HandlerFunc(handleSelectWorkspace)))
	mux.Handle("/api/workspace/profile/save", requireAuth(http.HandlerFunc(handleSaveProfile)))
	mux.Handle("/api/workspace/profile/load", requireAuth(http.HandlerFunc(handleLoadProfile)))
	mux.Handle("/api/workspace/sleep", requireAuth(http.HandlerFunc(handleWorkspaceSleep)))
	mux.Handle("/api/mocks/save", requireAuth(http.HandlerFunc(handleSaveMock)))
	mux.Handle("/api/mocks/list", requireAuth(http.HandlerFunc(handleListMocks)))
	mux.Handle("/api/compose/up", requireAuth(http.HandlerFunc(handleComposeUp)))
	mux.Handle("/api/compose/down", requireAuth(http.HandlerFunc(handleComposeDown)))
	mux.Handle("/api/services/toggle", requireAuth(http.HandlerFunc(handleToggleService)))
	mux.Handle("/api/services/stub", requireAuth(http.HandlerFunc(handleConfigureStub)))
	mux.Handle("/api/services/{name}/test", requireAuth(http.HandlerFunc(handleTestService)))
	mux.Handle("/api/chaos", requireAuth(http.HandlerFunc(handleConfigureChaos)))
	mux.Handle("/api/library/toggle-active", requireAuth(http.HandlerFunc(handleToggleActiveStub)))
	mux.Handle("/api/library/toggle-all", requireAuth(http.HandlerFunc(handleToggleAllStubs)))
	mux.Handle("/api/library/add", requireAuth(http.HandlerFunc(handleAddCustomProtocol)))
	mux.Handle("/api/export", requireAuth(http.HandlerFunc(handleExportMockCompose)))
	mux.Handle("/api/license/activate", requireAuth(http.HandlerFunc(handleLicenseActivate)))
	mux.Handle("/api/services/generate-mock", requireAuth(http.HandlerFunc(handleGenerateAIMock)))
	mux.Handle("/api/export/uml", requireAuth(http.HandlerFunc(handleExportUML)))
	mux.Handle("/api/logs/clear", requireAuth(http.HandlerFunc(handleClearLogs)))
	mux.HandleFunc("/api/auth/ca.crt", handleDownloadCACert)
	mux.HandleFunc("/api/workspace/readiness", handleWorkspaceReadiness)

	// Initialize Certificate Authority
	if err := initCA(); err != nil {
		log.Printf("Warning: Failed to initialize Root Certificate Authority: %v", err)
	}

	// Start connection logs synchronization background worker
	startLogSyncWorker()

	corsHandler := enableCORS(mux)
	listenAddr := os.Getenv("MOCKDOCK_BIND_ADDR")
	if listenAddr == "" {
		listenAddr = "127.0.0.1:11800"
	}
	log.Printf("MockDock daemon listening on http://%s...\n", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, corsHandler))
}

func handleLicenseStatus(w http.ResponseWriter, r *http.Request) {
	info := checkLicenseState()
	jsonResponse(w, http.StatusOK, info)
}

func handleDownloadCACert(w http.ResponseWriter, r *http.Request) {
	caCertPath := filepath.Join(GLOBAL_MOCKDOCK_DIR, "ca.crt")
	if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
		http.Error(w, "Certificate Authority not initialized", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", "attachment; filename=mockdock-ca.crt")
	http.ServeFile(w, r, caCertPath)
}

func isPortOccupied(port string) bool {
	conn, err := net.DialTimeout("tcp", "host.docker.internal:"+port, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return true // Connection succeeded, port is occupied on host
	}
	return false
}

func getHostPort(portStr string) string {
	parts := strings.Split(portStr, ":")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func handleWorkspaceReadiness(w http.ResponseWriter, r *http.Request) {
	dockerStatus := "success"
	dockerMsg := "Connected to Docker daemon."
	dockerDetail := ""
	containers, err := getDockerContainers()
	if err != nil {
		dockerStatus = "fail"
		dockerMsg = "Failed to connect to Docker daemon."
		dockerDetail = err.Error()
	}

	runningServices := make(map[string]bool)
	if containers != nil {
		for _, c := range containers {
			cMap, ok := c.(map[string]interface{})
			if ok && cMap["State"] == "running" {
				if svcName, ok := cMap["Service"].(string); ok {
					runningServices[svcName] = true
				}
			}
		}
	}

	portsStatus := "success"
	portsMsg := "All required host ports are available."
	portsDetail := ""
	var conflicts []map[string]interface{}

	stateMutex.Lock()
	for svcName, svc := range state.Services {
		if runningServices[svcName] {
			continue
		}
		for _, portMapping := range svc.Ports {
			hostPort := getHostPort(portMapping)
			if hostPort != "" && isPortOccupied(hostPort) {
				conflicts = append(conflicts, map[string]interface{}{
					"service": svcName,
					"port":    hostPort,
					"detail":  fmt.Sprintf("Host port %s is already in use by another process.", hostPort),
				})
			}
		}
	}
	
	rawCompose := state.RawCompose
	servicesCount := len(state.Services)
	activeStubsCount := 0
	for _, svc := range state.Services {
		if svc.Mocked {
			activeStubsCount++
		}
	}
	stateMutex.Unlock()

	if len(conflicts) > 0 {
		portsStatus = "fail"
		portsMsg = fmt.Sprintf("%d port conflict(s) detected.", len(conflicts))
	}

	composeStatus := "success"
	composeMsg := "Docker Compose configuration is valid."
	composeDetail := ""
	if rawCompose == "" {
		composeStatus = "fail"
		composeMsg = "Compose configuration is empty."
	} else {
		var composeMap map[string]interface{}
		if err := yaml.Unmarshal([]byte(rawCompose), &composeMap); err != nil {
			composeStatus = "fail"
			composeMsg = "Invalid YAML compose syntax."
			composeDetail = err.Error()
		} else if servicesCount == 0 {
			composeStatus = "fail"
			composeMsg = "No services defined in compose file."
		}
	}

	stubsStatus := "success"
	stubsMsg := "Stubs active and configured."
	stubsDetail := ""
	if activeStubsCount == 0 {
		stubsStatus = "warning"
		stubsMsg = "Running in pure Real Mode."
		stubsDetail = "No services are currently set to Ghost Mode. All network calls go to original live containers."
	}

	ready := dockerStatus == "success" && portsStatus == "success" && composeStatus == "success"

	response := map[string]interface{}{
		"ready": ready,
		"checks": map[string]interface{}{
			"docker": map[string]interface{}{
				"status":  dockerStatus,
				"message": dockerMsg,
				"detail":  dockerDetail,
			},
			"ports": map[string]interface{}{
				"status":    portsStatus,
				"message":   portsMsg,
				"detail":    portsDetail,
				"conflicts": conflicts,
			},
			"compose": map[string]interface{}{
				"status":  composeStatus,
				"message": composeMsg,
				"detail":  composeDetail,
			},
			"stubs": map[string]interface{}{
				"status":  stubsStatus,
				"message": stubsMsg,
				"detail":  stubsDetail,
			},
		},
	}

	jsonResponse(w, http.StatusOK, response)
}

func handleGetAuthToken(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://localhost:11800" && origin != "http://127.0.0.1:11800" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	token := DAEMON_AUTH_TOKEN
	gcfg, err := readGlobalConfig()
	if err == nil && gcfg != nil && gcfg.AuthToken != "" {
		token = gcfg.AuthToken
	}
	jsonResponse(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
		expectedToken := DAEMON_AUTH_TOKEN
		gcfg, err := readGlobalConfig()
		if err == nil && gcfg != nil && gcfg.AuthToken != "" {
			expectedToken = gcfg.AuthToken
		}
		if token == "" || token != expectedToken {
			jsonResponse(w, http.StatusUnauthorized, map[string]string{
				"detail": "Unauthorized: invalid or missing daemon authentication token.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleLicenseActivate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LicenseKey string `json:"license_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": "invalid request payload"})
		return
	}
	info, err := activateLicenseKey(body.LicenseKey)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]string{"detail": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, info)
}

func handleExportUML(w http.ResponseWriter, r *http.Request) {
	if !IS_RAD_ACTIVE {
		jsonResponse(w, http.StatusPaymentRequired, map[string]string{
			"detail": "MockDockRAD premium license is required to export topology as UML.",
		})
		return
	}
	
	stateMutex.Lock()
	defer stateMutex.Unlock()
	
	var sb strings.Builder
	sb.WriteString("@startuml\n")
	sb.WriteString("title " + state.ProjectLabel + " Topology Diagram\n\n")
	
	// Node definitions
	for name, svc := range state.Services {
		style := ""
		if svc.Mocked {
			style = " << (M,#FF5733) Ghost Stub >>"
		}
		sb.WriteString(fmt.Sprintf("node \"%s\"%s\n", name, style))
	}
	
	for name, extSvc := range state.ExternalServices {
		style := ""
		if extSvc.Mocked {
			style = " << (M,#C70039) External Stub >>"
		}
		sb.WriteString(fmt.Sprintf("cloud \"%s\"%s\n", name, style))
	}
	
	sb.WriteString("\n")
	
	// Connections
	for name, svc := range state.Services {
		var deps []string
		if svc.DependsOn != nil {
			if list, ok := svc.DependsOn.([]interface{}); ok {
				for _, d := range list {
					deps = append(deps, fmt.Sprintf("%v", d))
				}
			} else if m, ok := svc.DependsOn.(map[string]interface{}); ok {
				for k := range m {
					deps = append(deps, k)
				}
			}
		}
		for _, dep := range deps {
			sb.WriteString(fmt.Sprintf("[%s] --> [%s] : depends_on\n", name, dep))
		}
	}
	sb.WriteString("@enduml\n")
	
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

func parseLatency(l string) int {
	l = strings.TrimSuffix(strings.ToLower(l), "ms")
	val, _ := strconv.Atoi(l)
	return val
}

func parseLoss(l string) int {
	l = strings.TrimSuffix(l, "%")
	val, _ := strconv.Atoi(l)
	return val
}

func parseDuration(d string) time.Duration {
	d = strings.ToLower(d)
	unit := time.Second
	if strings.HasSuffix(d, "s") {
		d = strings.TrimSuffix(d, "s")
	} else if strings.HasSuffix(d, "m") {
		d = strings.TrimSuffix(d, "m")
		unit = time.Minute
	}
	val, _ := strconv.Atoi(d)
	if val <= 0 {
		val = 30
	}
	return time.Duration(val) * unit
}

func getDefaultPort(proto string) int {
	switch strings.ToLower(proto) {
	case "postgres":
		return 5432
	case "mysql":
		return 3306
	case "redis":
		return 6379
	case "mongodb":
		return 27017
	case "rabbitmq":
		return 5672
	default:
		return 80
	}
}

func initializeCLIState() {
	initPaths()
	loadSourcesFromDisk()
	composeContent := ""
	if body, err := os.ReadFile(COMPOSE_PATH); err == nil {
		composeContent = string(body)
	}
	initializeStateFromCompose(composeContent)
}

func handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if !IS_RAD_ACTIVE {
		jsonResponse(w, http.StatusPaymentRequired, map[string]string{
			"detail": "MockDockRAD premium license is required to view connection logs.",
		})
		return
	}

	events := loadConnectionLogEvents(100)
	list := make([]map[string]string, 0, len(events))
	for i, evt := range events {
		list = append(list, map[string]string{
			"id":        fmt.Sprintf("%d", i),
			"timestamp": evt.Timestamp,
			"source":    evt.Source,
			"target":    evt.Target,
			"protocol":  evt.Protocol,
			"event":     evt.Event,
			"status":    evt.Status,
			"latency":   evt.Latency,
			"req":       evt.Req,
			"resp":      evt.Resp,
		})
	}

	jsonResponse(w, http.StatusOK, list)
}

func handleClearLogs(w http.ResponseWriter, r *http.Request) {
	_ = os.Remove(connectionLogsFilePath())
	jsonResponse(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func installTestLabs() {
	fmt.Println("📥 Downloading MockDock Target Labs from GitHub...")
	resp, err := http.Get("https://github.com/MockDockapp/mockdock/archive/refs/heads/main.zip")
	if err != nil {
		log.Fatalf("❌ Failed to download target labs archive: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("❌ Received bad status code when downloading archive: %s", resp.Status)
	}

	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("❌ Failed to read archive data: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		log.Fatalf("❌ Failed to open zip archive: %v", err)
	}

	targetDir := "mockdock-labs"
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		log.Fatalf("❌ Failed to create target directory %s: %v", targetDir, err)
	}

	labsCount := 0
	for _, file := range reader.File {
		parts := strings.Split(file.Name, "/")
		idx := -1
		for i, part := range parts {
			if part == "mockdock-labs" {
				idx = i
				break
			}
		}
		if idx == -1 || idx == len(parts)-1 {
			continue
		}

		relParts := parts[idx:]
		destPath := filepath.Join(relParts...)

		if file.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(destPath), 0755)

		rc, err := file.Open()
		if err != nil {
			log.Fatalf("❌ Failed to open zip file member %s: %v", file.Name, err)
		}

		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			rc.Close()
			log.Fatalf("❌ Failed to create local file %s: %v", destPath, err)
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			log.Fatalf("❌ Failed to write local file %s: %v", destPath, err)
		}
		labsCount++
	}

	if labsCount == 0 {
		log.Fatalf("❌ Did not find any mockdock-labs/ files in the downloaded zip archive. Please verify repository path.")
	}

	fmt.Printf("✅ Successfully installed MockDock Target Labs and lessons in './%s'! (extracted %d files)\n", targetDir, labsCount)
}
