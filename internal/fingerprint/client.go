package fingerprint

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type protocolError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type protocolMessage struct {
	ID        int64           `json:"id,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *protocolError  `json:"error,omitempty"`
}

type callResult struct {
	result json.RawMessage
	err    error
}

type lockedWebsocket struct {
	connection net.Conn
	reader     *bufio.Reader
	writeMu    sync.Mutex
}

func (socket *lockedWebsocket) Read(buffer []byte) (int, error) {
	// gobwas/ws only returns a buffered reader when the HTTP upgrade left
	// unread bytes behind. A clean handshake legitimately returns nil, in
	// which case frames must be read directly from the TCP connection.
	if socket.reader == nil {
		return socket.connection.Read(buffer)
	}
	return socket.reader.Read(buffer)
}

func (socket *lockedWebsocket) Write(buffer []byte) (int, error) {
	socket.writeMu.Lock()
	defer socket.writeMu.Unlock()
	return socket.connection.Write(buffer)
}

func (socket *lockedWebsocket) writeClientText(payload []byte) error {
	socket.writeMu.Lock()
	defer socket.writeMu.Unlock()
	return wsutil.WriteClientText(socket.connection, payload)
}

type cdpClient struct {
	socket *lockedWebsocket
	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan callResult
	events  chan protocolMessage
	done    chan struct{}
	closed  sync.Once
	readWG  sync.WaitGroup
}

func newCDPClient(ctx context.Context, websocketURL string) (*cdpClient, error) {
	connection, reader, _, err := ws.DefaultDialer.Dial(ctx, websocketURL)
	if err != nil {
		return nil, fmt.Errorf("dial DevTools websocket: %w", err)
	}
	client := &cdpClient{
		socket:  &lockedWebsocket{connection: connection, reader: reader},
		pending: make(map[int64]chan callResult), events: make(chan protocolMessage, 256), done: make(chan struct{}),
	}
	client.readWG.Add(1)
	go client.readLoop()
	return client, nil
}

func (client *cdpClient) Call(ctx context.Context, sessionID, method string, params, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := client.nextID.Add(1)
	response := make(chan callResult, 1)
	client.mu.Lock()
	select {
	case <-client.done:
		client.mu.Unlock()
		return errors.New("DevTools connection is closed")
	default:
	}
	client.pending[id] = response
	client.mu.Unlock()

	command := struct {
		ID        int64  `json:"id"`
		SessionID string `json:"sessionId,omitempty"`
		Method    string `json:"method"`
		Params    any    `json:"params,omitempty"`
	}{ID: id, SessionID: sessionID, Method: method, Params: params}
	payload, err := json.Marshal(command)
	if err == nil {
		err = client.socket.writeClientText(payload)
	}
	if err != nil {
		client.removePending(id)
		return fmt.Errorf("send CDP %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		client.removePending(id)
		return ctx.Err()
	case <-client.done:
		client.removePending(id)
		return errors.New("DevTools connection closed while waiting for " + method)
	case received := <-response:
		if received.err != nil {
			return fmt.Errorf("CDP %s: %w", method, received.err)
		}
		if result != nil && len(received.result) > 0 {
			if err := json.Unmarshal(received.result, result); err != nil {
				return fmt.Errorf("decode CDP %s response: %w", method, err)
			}
		}
		return nil
	}
}

func (client *cdpClient) removePending(id int64) {
	client.mu.Lock()
	delete(client.pending, id)
	client.mu.Unlock()
}

func (client *cdpClient) readLoop() {
	defer client.readWG.Done()
	for {
		payload, err := wsutil.ReadServerText(client.socket)
		if err != nil {
			client.shutdown(fmt.Errorf("read DevTools websocket: %w", err))
			return
		}
		var message protocolMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		if message.ID != 0 {
			client.mu.Lock()
			response := client.pending[message.ID]
			delete(client.pending, message.ID)
			client.mu.Unlock()
			if response != nil {
				var resultError error
				if message.Error != nil {
					resultError = fmt.Errorf("protocol error %d: %s", message.Error.Code, message.Error.Message)
				}
				response <- callResult{result: message.Result, err: resultError}
			}
			continue
		}
		if message.Method != "" {
			select {
			case <-client.done:
				return
			case client.events <- message:
			}
		}
	}
}

func (client *cdpClient) shutdown(cause error) {
	client.closed.Do(func() {
		close(client.done)
		client.mu.Lock()
		for id, response := range client.pending {
			response <- callResult{err: cause}
			delete(client.pending, id)
		}
		client.mu.Unlock()
	})
}

func (client *cdpClient) Close() error {
	err := client.socket.connection.Close()
	client.shutdown(errors.New("DevTools connection closed"))
	client.readWG.Wait()
	return err
}
