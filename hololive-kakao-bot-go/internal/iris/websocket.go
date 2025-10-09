package iris

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type MessageCallback func(message *Message)

type StateCallback func(state WebSocketState)

type callbackEntry struct {
	id       int
	callback MessageCallback
}

type stateCallbackEntry struct {
	id       int
	callback StateCallback
}

type WebSocket struct {
	wsURL                string
	conn                 *websocket.Conn
	state                WebSocketState
	stateMu              sync.RWMutex
	messageCallbacks     []callbackEntry
	stateCallbacks       []stateCallbackEntry
	nextCallbackID       int
	callbacksMu          sync.RWMutex
	reconnectAttempts    int
	maxReconnectAttempts int
	reconnectDelay       time.Duration
	logger               *zap.Logger
	stopCh               chan struct{}
	doneCh               chan struct{}
	stopOnce             sync.Once
	listenerWg           sync.WaitGroup
}

func NewWebSocket(wsURL string, maxReconnectAttempts int, reconnectDelay time.Duration, logger *zap.Logger) *WebSocket {
	return &WebSocket{
		wsURL:                wsURL,
		state:                WSStateDisconnected,
		maxReconnectAttempts: maxReconnectAttempts,
		reconnectDelay:       reconnectDelay,
		logger:               logger,
		stopCh:               make(chan struct{}),
		doneCh:               make(chan struct{}),
		messageCallbacks:     make([]callbackEntry, 0),
		stateCallbacks:       make([]stateCallbackEntry, 0),
		nextCallbackID:       1,
	}
}

func (ws *WebSocket) Connect(ctx context.Context) error {
	ws.stateMu.Lock()
	if ws.state == WSStateConnected || ws.state == WSStateConnecting {
		ws.stateMu.Unlock()
		ws.logger.Warn("WebSocket already connected or connecting")
		return nil
	}
	ws.stateMu.Unlock()

	ws.setState(WSStateConnecting)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.DialContext(ctx, ws.wsURL, nil)
	if err != nil {
		ws.logger.Error("Failed to connect WebSocket", zap.Error(err))
		ws.setState(WSStateFailed)
		ws.scheduleReconnect(ctx)
		return err
	}

	ws.conn = conn
	ws.setState(WSStateConnected)
	ws.reconnectAttempts = 0

	ws.logger.Info("WebSocket connected", zap.String("url", ws.wsURL))

	ws.listenerWg.Add(1)
	go ws.listen(ctx)

	return nil
}

func (ws *WebSocket) listen(ctx context.Context) {
	defer ws.listenerWg.Done()
	defer ws.logger.Info("WebSocket listener stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ws.stopCh:
			return
		default:
			if ws.conn == nil {
				return
			}

			_, msgBytes, err := ws.conn.ReadMessage()
			if err != nil {
				ws.logger.Error("WebSocket read error", zap.Error(err))
				ws.setState(WSStateDisconnected)
				ws.scheduleReconnect(ctx)
				return
			}

			ws.handleMessage(msgBytes)
		}
	}
}

func (ws *WebSocket) handleMessage(data []byte) {
	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		dataStr := string(data)
		if len(dataStr) > 200 {
			dataStr = dataStr[:200]
		}
		ws.logger.Error("Failed to parse message",
			zap.Error(err),
			zap.String("data", dataStr),
		)
		return
	}

	if message.JSON != nil {
	}

	ws.callbacksMu.RLock()
	callbacks := make([]callbackEntry, len(ws.messageCallbacks))
	copy(callbacks, ws.messageCallbacks)
	ws.callbacksMu.RUnlock()

	for _, entry := range callbacks {
		entry.callback(&message)
	}
}

func (ws *WebSocket) scheduleReconnect(ctx context.Context) {
	ws.reconnectAttempts++

	if ws.reconnectAttempts > ws.maxReconnectAttempts {
		ws.logger.Error("Max reconnect attempts reached",
			zap.Int("attempts", ws.reconnectAttempts),
		)
		ws.setState(WSStateFailed)
		return
	}

	ws.setState(WSStateReconnecting)

	ws.logger.Info("Scheduling reconnect",
		zap.Int("attempt", ws.reconnectAttempts),
		zap.Int("max", ws.maxReconnectAttempts),
		zap.Duration("delay", ws.reconnectDelay),
	)

	go func() {
		select {
		case <-time.After(ws.reconnectDelay):
			if err := ws.Connect(ctx); err != nil {
				ws.logger.Error("Reconnect failed", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}()
}

func (ws *WebSocket) OnMessage(callback MessageCallback) func() {
	ws.callbacksMu.Lock()
	id := ws.nextCallbackID
	ws.nextCallbackID++
	ws.messageCallbacks = append(ws.messageCallbacks, callbackEntry{
		id:       id,
		callback: callback,
	})
	ws.callbacksMu.Unlock()

	return func() {
		ws.callbacksMu.Lock()
		defer ws.callbacksMu.Unlock()
		for i, entry := range ws.messageCallbacks {
			if entry.id == id {
				ws.messageCallbacks = append(ws.messageCallbacks[:i], ws.messageCallbacks[i+1:]...)
				break
			}
		}
	}
}

func (ws *WebSocket) OnStateChange(callback StateCallback) func() {
	ws.callbacksMu.Lock()
	id := ws.nextCallbackID
	ws.nextCallbackID++
	ws.stateCallbacks = append(ws.stateCallbacks, stateCallbackEntry{
		id:       id,
		callback: callback,
	})
	ws.callbacksMu.Unlock()

	return func() {
		ws.callbacksMu.Lock()
		defer ws.callbacksMu.Unlock()
		for i, entry := range ws.stateCallbacks {
			if entry.id == id {
				ws.stateCallbacks = append(ws.stateCallbacks[:i], ws.stateCallbacks[i+1:]...)
				break
			}
		}
	}
}

func (ws *WebSocket) setState(newState WebSocketState) {
	ws.stateMu.Lock()
	oldState := ws.state
	ws.state = newState
	ws.stateMu.Unlock()

	if oldState != newState {
		ws.logger.Info("WebSocket state changed",
			zap.String("from", oldState.String()),
			zap.String("to", newState.String()),
		)

		ws.callbacksMu.RLock()
		callbacks := make([]stateCallbackEntry, len(ws.stateCallbacks))
		copy(callbacks, ws.stateCallbacks)
		ws.callbacksMu.RUnlock()

		for _, entry := range callbacks {
			entry.callback(newState)
		}
	}
}

func (ws *WebSocket) GetState() WebSocketState {
	ws.stateMu.RLock()
	defer ws.stateMu.RUnlock()
	return ws.state
}

func (ws *WebSocket) IsConnected() bool {
	return ws.GetState() == WSStateConnected
}

func (ws *WebSocket) Disconnect() error {
	ws.stopOnce.Do(func() {
		close(ws.stopCh)
	})

	if ws.conn != nil {
		if err := ws.conn.Close(); err != nil {
			ws.logger.Error("Failed to close WebSocket", zap.Error(err))
			return err
		}
		ws.conn = nil
	}

	ws.reconnectAttempts = 0
	ws.setState(WSStateDisconnected)
	ws.logger.Info("WebSocket disconnected")

	done := make(chan struct{})
	go func() {
		ws.listenerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		ws.logger.Info("Listener stopped cleanly")
	case <-time.After(5 * time.Second):
		ws.logger.Warn("Timeout waiting for listener to stop")
	}

	return nil
}

func (ws *WebSocket) RemoveAllListeners() {
	ws.callbacksMu.Lock()
	defer ws.callbacksMu.Unlock()
	ws.messageCallbacks = make([]callbackEntry, 0)
	ws.stateCallbacks = make([]stateCallbackEntry, 0)
}
