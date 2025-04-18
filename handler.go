package websockethandler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type HandlerFunc func(context.Context, WsFuncData) (WsFuncData, error)

type wsHandlerTree struct {
	main     HandlerFunc
	parent   *wsHandlerTree
	children *wsHandlerTree
}

type WsFuncData struct {
	Client  interface{}
	Payload MessagePayload
}

// MessagePayload represents the structure of incoming WebSocket messages
type MessagePayload struct {
	Event     string      `json:"event"`
	Data      interface{} `json:"data,omitempty"`
	Status    string      `json:"status,omitempty"`
	Broadcast bool        `json:"-"`
}

type WsFunc struct {
	Event  string
	Status string
}

// Handler for functions
// Must support automatic error logging
// And message return to the user in the channel
type WsHandler interface {
	Handle(meta WsFunc, f HandlerFunc, parent ...HandlerFunc) WsHandler
	CallFunc(ctx context.Context, meta WsFunc, data WsFuncData) (WsFuncData, error)
	CallPipelineFunc(ctx context.Context, meta WsFunc, data WsFuncData, ch chan MessagePayload) error
	SetLogLevel(level string) WsHandler
	GetError() error
}

type wsHandler struct {
	mutex    sync.RWMutex
	fun      map[WsFunc]HandlerFunc
	funcTree map[string]*wsHandlerTree

	// Logging
	logger   stdLogger
	logLevel level
	err      error
}

func NewHandler(logger stdLogger) WsHandler {
	handler := &wsHandler{
		fun:      make(map[WsFunc]HandlerFunc),
		funcTree: make(map[string]*wsHandlerTree),
		logger:   logger,
		logLevel: infoLevel,
	}
	handler.log(
		infoLevel,
		fmt.Errorf("new handler is registered"),
	)
	return handler
}

func (h *wsHandler) log(lvl level, event error, data ...interface{}) {
	if h.logLevel >= lvl {
		logMsg := strLog{
			UUID:   uuid.NewString(),
			Event:  fmt.Errorf("%w", event),
			Level:  lvl,
			Module: "websockethandler",
			Body:   data,
		}
		h.logger.Print(logMsg)
	}
}

func (h *wsHandler) GetError() error {
	return h.err
}

// Setting the logging level
func (h *wsHandler) SetLogLevel(level string) WsHandler {
	if h.err == nil {
		lvl, err := ParseLevel(level)
		if err != nil {
			h.err = fmt.Errorf("%w:%s", err, "SetLogLevel")
		} else {
			h.logLevel = lvl
			h.log(infoLevel,
				fmt.Errorf("change log level to %s", level))
		}
	}
	return h
}

// Function registration
func (h *wsHandler) Handle(meta WsFunc, f HandlerFunc, parent ...HandlerFunc) WsHandler {
	if h.err == nil {
		h.mutex.Lock()
		defer h.mutex.Unlock()
		if _, ok := h.fun[meta]; ok {
			h.err = fmt.Errorf("func with current params has been registered")
		} else {
			if len(parent) > 0 {
				parentFunc := parent[0]
				keyMain := fmt.Sprintf("%#v", f)
				mainHandlerTree, ok := h.funcTree[keyMain]
				if ok {
					if mainHandlerTree.children != nil {
						h.err = fmt.Errorf("the current function has a child function declaration")
						return h
					}
				} else {
					mainHandlerTree = &wsHandlerTree{main: f}
					h.funcTree[keyMain] = mainHandlerTree
				}

				keyParent := fmt.Sprintf("%#v", parentFunc)
				if parentHandlerTree, ok := h.funcTree[keyParent]; ok {
					if parentHandlerTree.children != nil {
						h.err = fmt.Errorf("the parent function has a child function declaration:%s:%s:%s", keyMain, keyParent, getFunctionName())
						return h
					}
					parentHandlerTree.children = mainHandlerTree
					mainHandlerTree.parent = parentHandlerTree
				} else {
					h.err = fmt.Errorf("there is no registered parent function:%s:%s:%s", keyMain, keyParent, getFunctionName())
					return h
				}
			} else {
				keyMain := fmt.Sprintf("%#v", f)
				if _, ok := h.funcTree[keyMain]; ok {
					h.err = fmt.Errorf("this function is declared:%s:%s", keyMain, getFunctionName())
					return h
				} else {
					h.funcTree[keyMain] = &wsHandlerTree{main: f}
				}
			}
			h.fun[meta] = f
		}
	}
	return h
}

// Calling an event in pipeline mode with self-sending information to a buffered channel
func (h *wsHandler) CallPipelineFunc(ctx context.Context, meta WsFunc, data WsFuncData, ch chan MessagePayload) error {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	h.log(
		debugLevel,
		fmt.Errorf("in:%v:%v:%s", meta, data, getFunctionName()),
	)
	if f, ok := h.fun[meta]; ok {
		keyMain := fmt.Sprintf("%#v", f)
		if f, ok := h.funcTree[keyMain]; ok {
			for {
				ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Second*30)
				defer cancel()

				d := h.shell(f.main, ctxWithTimeout, data)
				ch <- d.Payload
				if d.Payload.Status == ErrorLevel {
					break
				}

				if f.children != nil {
					f = f.children
				} else {
					break
				}
			}
		} else {
			ch <- MessagePayload{Event: data.Payload.Event, Status: ErrorLevel}
			return fmt.Errorf("func with current params has not been registered for pipeline:%v:%s", meta, getFunctionName())
		}
	} else {
		ch <- MessagePayload{Event: data.Payload.Event, Status: ErrorLevel}
		return fmt.Errorf("func with current params has not been registered:%v:%s", meta, getFunctionName())
	}
	return nil
}

func (h *wsHandler) CallFunc(ctx context.Context, meta WsFunc, data WsFuncData) (WsFuncData, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	h.log(
		debugLevel,
		fmt.Errorf("in:%v:%v:%s", meta, data, getFunctionName()),
	)
	if f, ok := h.fun[meta]; ok {
		d := h.shell(f, ctx, data)
		h.log(
			debugLevel,
			fmt.Errorf("out:%v:%v:%s", meta, d, getFunctionName()),
		)
		return d, nil
	} else {
		return WsFuncData{Payload: MessagePayload{Event: data.Payload.Event, Status: ErrorLevel}},
			fmt.Errorf("func with current params has not been registered:%v:%s", meta, getFunctionName())
	}
}

func (h *wsHandler) shell(f HandlerFunc, ctx context.Context, data WsFuncData) WsFuncData {
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				h.log(
					errorLevel,
					fmt.Errorf("%w:%s", ctx.Err(), getFunctionName()),
					data.Payload,
					data.Client,
				)
				return WsFuncData{
					Client: data.Client,
					Payload: MessagePayload{
						Event:  data.Payload.Event,
						Status: ErrorLevel,
						Data:   "timeout reached",
					},
				}
			}
		case <-time.After(time.Millisecond):
			d, err := f(ctx, data)
			if err != nil {
				h.log(
					errorLevel,
					fmt.Errorf("%w:%s", err, getFunctionName()),
					data.Payload,
					data.Client,
				)
			}
			return d
		}
	}
}
