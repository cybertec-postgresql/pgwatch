package log

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveSubscriber(t *testing.T) {
	hook := NewBrokerHook(context.Background(), "info")
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
