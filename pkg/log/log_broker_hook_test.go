package log

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type errorFormatter struct{}

func (f *errorFormatter) Format(*logrus.Entry) ([]byte, error) {
	return nil, assert.AnError
}

func TestFormatterError(t *testing.T) {
	hook := NewBrokerHook(t.Context(), "info")
	hook.SetBrokerFormatter(&errorFormatter{})
	entry := logrus.NewEntry(logrus.New())
	entry.Message = "hello broker"
	entry.Level = logrus.InfoLevel
	assert.Error(t, hook.Fire(entry))
}

func TestRemoveSubscriber(t *testing.T) {
	hook := NewBrokerHook(t.Context(), "info")
	msgChan1 := make(MessageChanType)
	msgChan2 := make(MessageChanType)
	hook.AddSubscriber(msgChan1)
	hook.AddSubscriber(msgChan2)
	assert.Equal(t, 2, len(hook.subscribers))

	// Remove the first subscriber
	hook.RemoveSubscriber(msgChan1)
	assert.Equal(t, 1, len(hook.subscribers))

	// Remove the last subscriber, this is where the "index out of range" error occurs
	hook.RemoveSubscriber(msgChan2)
	assert.Equal(t, 0, len(hook.subscribers))
}

func TestFire_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	hook := NewBrokerHook(ctx, "info")
	err := hook.Fire(logrus.NewEntry(logrus.New()))
	assert.NoError(t, err)
}

func TestFire_DeliverToSubscriber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		hook := NewBrokerHook(ctx, "info")
		msgChan := make(MessageChanType, 1)
		hook.AddSubscriber(msgChan)

		entry := logrus.NewEntry(logrus.New())
		entry.Message = "hello broker"
		entry.Level = logrus.InfoLevel
		assert.NoError(t, hook.Fire(entry))

		// Wait for the poll goroutine to drain input and deliver to msgChan.
		synctest.Wait()

		select {
		case msg := <-msgChan:
			assert.Contains(t, string(msg), "hello broker")
		default:
			t.Fatal("message not delivered")
		}

		cancel()        // signal poll to exit
		synctest.Wait() // wait for poll goroutine to return
	})
}

func TestFire_HighLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Construct without starting poll so the input channel stays full.
		hook := &BrokerHook{
			highLoadTimeout: time.Millisecond,
			input:           make(chan MessageType, 1),
			ctx:             t.Context(),
			level:           "info",
			mu:              &sync.Mutex{},
			formatter:       defaultFormatter,
		}
		hook.input <- "fill" // fill to capacity; no poll goroutine to drain it

		entry := logrus.NewEntry(logrus.New())
		entry.Message = "to be dropped"
		// The test goroutine is the only goroutine in the bubble.
		// Fire blocks on time.After; fake time advances and takes the drop branch.
		assert.NoError(t, hook.Fire(entry))
	})
}

func TestSend_NoSubscribersIsNoop(t *testing.T) {
	hook := NewBrokerHook(t.Context(), "info")
	// No subscribers — send must return without blocking or panicking.
	hook.send("msg")
}

func TestSend_FullSubscriberDrops(t *testing.T) {
	hook := NewBrokerHook(t.Context(), "info")
	// Unbuffered channel with no reader — send must take the default branch and not block.
	msgChan := make(MessageChanType)
	hook.AddSubscriber(msgChan)
	hook.send("dropped")
}

func TestSetBrokerFormatter_NilResetsDefault(t *testing.T) {
	hook := NewBrokerHook(t.Context(), "info")
	hook.SetBrokerFormatter(&logrus.JSONFormatter{})
	hook.SetBrokerFormatter(nil)
	assert.Equal(t, defaultFormatter, hook.formatter)
}

func TestLevels(t *testing.T) {
	cases := []struct {
		level    string
		expected []logrus.Level
	}{
		{"none", []logrus.Level{}},
		{"debug", logrus.AllLevels},
		{"info", []logrus.Level{logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel, logrus.InfoLevel}},
		{"error", []logrus.Level{logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel}},
	}
	for _, tc := range cases {
		hook := NewBrokerHook(t.Context(), tc.level)
		assert.Equal(t, tc.expected, hook.Levels(), "level=%s", tc.level)
	}
}
