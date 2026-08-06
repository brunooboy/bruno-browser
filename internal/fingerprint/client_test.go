package fingerprint

import (
	"io"
	"net"
	"testing"
)

func TestLockedWebsocketReadsConnectionWhenHandshakeReaderIsNil(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()

	written := make(chan error, 1)
	go func() {
		_, err := serverConnection.Write([]byte("ok"))
		written <- err
	}()

	socket := &lockedWebsocket{connection: clientConnection}
	payload := make([]byte, 2)
	if _, err := io.ReadFull(socket, payload); err != nil {
		t.Fatalf("read direct websocket connection: %v", err)
	}
	if string(payload) != "ok" {
		t.Fatalf("unexpected payload %q", payload)
	}
	if err := <-written; err != nil {
		t.Fatalf("write test payload: %v", err)
	}
}
