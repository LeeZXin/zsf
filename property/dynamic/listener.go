package dynamic

import "github.com/LeeZXin/zsf/logger"

type EventType int

const (
	PutEventType EventType = iota + 1
	DeleteEventType
)

type Listener func(EventType, []byte)

var (
	listenerMap = make(map[string]Listener)
)

// RegisterListener 注册配置监听 not thread safe
func RegisterListener(key string, listener Listener) {
	if key != "" && listener != nil {
		_, b := listenerMap[key]
		if b {
			logger.Logger.Fatalf("dynamic property register listener duplicated key: %v", key)
		}
		listenerMap[key] = listener
	}
}

// notifyListener 通知监听
func notifyListener(key string, val []byte, eventType EventType) {
	listener, b := listenerMap[key]
	if b {
		go listener(eventType, val)
	}
}
