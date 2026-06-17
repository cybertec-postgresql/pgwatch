package log

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// MessageType represents the format of the message
type MessageType string

// MessageChanType represents the format of the message channel
type MessageChanType chan MessageType

// BrokerHook is the implementation of the logrus hook for publishing logs to subscribers
type BrokerHook struct {
	highLoadTimeout time.Duration     // wait this amount of time before skip log entry
	subscribers     []MessageChanType //
	input           chan MessageType
	ctx             context.Context
	level           string
	mu              *sync.Mutex
	formatter       logrus.Formatter
}

const cacheLimit = 512
const highLoadLimit = 200 * time.Millisecond

// NewBrokerHook creates a LogHook to be added to an instance of logger
func NewBrokerHook(ctx context.Context, level string) *BrokerHook {
	l := &BrokerHook{
		highLoadTimeout: highLoadLimit,
		input:           make(chan MessageType, cacheLimit),
		ctx:             ctx,
		level:           level,
		mu:              new(sync.Mutex),
		formatter:       defaultFormatter,
	}
	go l.poll(l.input)
	return l
}

// AddSubscriber adds receiving channel to the subscription
func (hook *BrokerHook) AddSubscriber(msgCh MessageChanType) {
	hook.mu.Lock()
	defer hook.mu.Unlock()
	hook.subscribers = append(hook.subscribers, msgCh)
}

// RemoveSubscriber deletes receiving channel from the subscription
func (hook *BrokerHook) RemoveSubscriber(msgCh MessageChanType) {
	hook.mu.Lock()
	defer hook.mu.Unlock()
	hook.subscribers = slices.DeleteFunc(hook.subscribers, func(E MessageChanType) bool {
		return E == msgCh
	})
}

var defaultFormatter = &logrus.TextFormatter{DisableColors: true}

// SetBrokerFormatter sets the format that will be used by hook.
func (hook *BrokerHook) SetBrokerFormatter(formatter logrus.Formatter) {
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if formatter == nil {
		hook.formatter = defaultFormatter
	} else {
		hook.formatter = formatter
	}
}

// Fire adds logrus log message to the internal queue for processing
func (hook *BrokerHook) Fire(entry *logrus.Entry) error {
	if hook.ctx.Err() != nil {
		return nil
	}
	hook.mu.Lock()
	f := hook.formatter
	hook.mu.Unlock()
	raw, err := f.Format(entry)
	if err != nil {
		return err
	}
	select {
	case hook.input <- MessageType(raw):
		// entry sent
	case <-time.After(hook.highLoadTimeout):
		// entry dropped due to a huge load, check stdout or file for detailed log
	}
	return nil
}

// Levels returns the available logging levels
func (hook *BrokerHook) Levels() []logrus.Level {
	switch hook.level {
	case "none":
		return []logrus.Level{}
	case "debug":
		return logrus.AllLevels
	case "info":
		return []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
			logrus.InfoLevel,
		}
	default:
		return []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
		}
	}
}

// poll checks for incoming messages and caches them internally
// until either a maximum amount is reached, or a timeout occurs.
func (hook *BrokerHook) poll(input <-chan MessageType) {
	for {
		select {
		case <-hook.ctx.Done(): //check context with high priority
			return
		case msg := <-input:
			hook.send(msg)
		}
	}
}

// send sends out a pre-formatted message to all subscribers
func (hook *BrokerHook) send(msg MessageType) {
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.subscribers) == 0 {
		return
	}
	for _, subscriber := range hook.subscribers {
		select {
		case subscriber <- msg:
		default:
			//no time to wait
		}
	}
}
