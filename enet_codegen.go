// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ENet machine code generation using gcc/tcc
//
// Strategy:
// 1. Write C code that uses ENet (enet.h is header-only with ENET_IMPLEMENTATION)
// 2. Compile with gcc/tcc to object file
// 3. Extract machine code and embed into executable

type ENetCodeGenerator struct {
	arch string
}

func NewENetCodeGenerator(arch string) *ENetCodeGenerator {
	return &ENetCodeGenerator{arch: arch}
}

// CompileENetFunction compiles C code with ENet and returns machine code
func (eg *ENetCodeGenerator) CompileENetFunction(functionName, cCode string) ([]byte, error) {
	tempDir, err := os.MkdirTemp("", "enet_compile_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy enet.h to temp directory
	enetHeader, err := os.ReadFile("enet.h")
	if err != nil {
		return nil, fmt.Errorf("failed to read enet.h: %v", err)
	}
	enetHeaderPath := filepath.Join(tempDir, "enet.h")
	if err := os.WriteFile(enetHeaderPath, enetHeader, 0644); err != nil {
		return nil, fmt.Errorf("failed to write enet.h: %v", err)
	}

	// Write C source with ENET_IMPLEMENTATION
	cFile := filepath.Join(tempDir, "enet_func.c")
	fullCode := fmt.Sprintf(`
#define ENET_IMPLEMENTATION
#include "enet.h"
#include <string.h>
#include <stdio.h>
#include <stdlib.h>

%s
`, cCode)

	if err := os.WriteFile(cFile, []byte(fullCode), 0644); err != nil {
		return nil, fmt.Errorf("failed to write C file: %v", err)
	}

	// Compile to object file
	objFile := filepath.Join(tempDir, "enet_func.o")
	cmd := exec.Command("gcc",
		"-c",
		"-O2",
		"-fno-asynchronous-unwind-tables",
		"-fno-stack-protector",
		"-mno-red-zone",
		"-fPIC",
		"-I", tempDir,
		"-o", objFile,
		cFile,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compile failed: %v\nOutput: %s", err, string(output))
	}

	// Extract .text section as raw machine code
	binFile := filepath.Join(tempDir, "enet_func.bin")
	cmd = exec.Command("objcopy",
		"-O", "binary",
		"--only-section=.text",
		objFile,
		binFile,
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("extract failed: %v\nOutput: %s", err, string(output))
	}

	machineCode, err := os.ReadFile(binFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read machine code: %v", err)
	}

	return machineCode, nil
}

// GenerateENetSendReceive generates complete send/receive machine code
func (eg *ENetCodeGenerator) GenerateENetSendReceive() ([]byte, []byte, error) {
	// Send function
	sendCode := `
int vibe67_enet_send(const char *address_str, const char *message, size_t len) {
    // Initialize ENet if needed
    static int enet_initialized = 0;
    if (!enet_initialized) {
        if (enet_initialize() != 0) return -1;
        enet_initialized = 1;
    }
    
    // Parse address (format: ":port" or "host:port")
    char host_buf[256] = "127.0.0.1";
    int port = 0;
    
    const char *colon = strchr(address_str, ':');
    if (colon == NULL) return -1;
    
    if (colon != address_str) {
        size_t host_len = colon - address_str;
        if (host_len >= sizeof(host_buf)) return -1;
        strncpy(host_buf, address_str, host_len);
        host_buf[host_len] = 0;
    }
    port = atoi(colon + 1);
    
    // Create client host
    ENetHost *client = enet_host_create(NULL, 1, 2, 0, 0);
    if (client == NULL) return -1;
    
    // Set up address
    ENetAddress address;
    if (enet_address_set_host(&address, host_buf) != 0) {
        enet_host_destroy(client);
        return -1;
    }
    address.port = port;
    
    // Connect to peer
    ENetPeer *peer = enet_host_connect(client, &address, 2, 0);
    if (peer == NULL) {
        enet_host_destroy(client);
        return -1;
    }
    
    // Wait for connection
    ENetEvent event;
    int connected = 0;
    for (int i = 0; i < 30; i++) {
        if (enet_host_service(client, &event, 100) > 0 && 
            event.type == ENET_EVENT_TYPE_CONNECT) {
            connected = 1;
            break;
        }
    }
    
    if (!connected) {
        enet_peer_reset(peer);
        enet_host_destroy(client);
        return -1;
    }
    
    // Create and send packet
    ENetPacket *packet = enet_packet_create(message, len, ENET_PACKET_FLAG_RELIABLE);
    if (packet == NULL) {
        enet_peer_disconnect(peer, 0);
        enet_host_destroy(client);
        return -1;
    }
    
    enet_peer_send(peer, 0, packet);
    enet_host_flush(client);
    
    // Disconnect
    enet_peer_disconnect(peer, 0);
    for (int i = 0; i < 10; i++) {
        if (enet_host_service(client, &event, 100) > 0 && 
            event.type == ENET_EVENT_TYPE_DISCONNECT) {
            break;
        }
    }
    
    enet_host_destroy(client);
    return len;
}
`

	// Receive function
	recvCode := `
typedef struct {
    char message[4096];
    size_t length;
    int success;
} ENetRecvResult;

void vibe67_enet_receive(const char *bind_address, ENetRecvResult *result, int timeout_ms) {
    result->success = 0;
    result->length = 0;
    
    // Initialize ENet if needed
    static int enet_initialized = 0;
    if (!enet_initialized) {
        if (enet_initialize() != 0) return;
        enet_initialized = 1;
    }
    
    // Parse bind address
    int port = 0;
    const char *colon = strchr(bind_address, ':');
    if (colon != NULL) {
        port = atoi(colon + 1);
    }
    
    // Set up server address
    ENetAddress address;
    address.host = ENET_HOST_ANY;
    address.port = port;
    
    // Create server host
    ENetHost *server = enet_host_create(&address, 32, 2, 0, 0);
    if (server == NULL) return;
    
    // Wait for event
    ENetEvent event;
    if (enet_host_service(server, &event, timeout_ms) > 0) {
        if (event.type == ENET_EVENT_TYPE_RECEIVE) {
            result->length = event.packet->dataLength;
            if (result->length > sizeof(result->message)) {
                result->length = sizeof(result->message);
            }
            memcpy(result->message, event.packet->data, result->length);
            result->success = 1;
            enet_packet_destroy(event.packet);
        }
    }
    
    enet_host_destroy(server);
}
`

	sendMachineCode, err := eg.CompileENetFunction("vibe67_enet_send", sendCode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile send: %v", err)
	}

	recvMachineCode, err := eg.CompileENetFunction("vibe67_enet_receive", recvCode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile receive: %v", err)
	}

	return sendMachineCode, recvMachineCode, nil
}









