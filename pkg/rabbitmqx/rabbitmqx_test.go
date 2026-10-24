package rabbitmqx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeDefaults(t *testing.T) {
	cfg, err := Config{}.Normalize()

	require.NoError(t, err)
	require.Equal(t, DefaultConfig().URL, cfg.URL)
	require.Equal(t, "xiaolanshu.events", cfg.Exchange)
	require.Equal(t, "topic", cfg.ExchangeType)
	require.Equal(t, "xiaolanshu.order.created", cfg.Queue)
	require.Equal(t, []string{"order.created"}, cfg.RoutingKeys)
	require.Equal(t, 10, cfg.PrefetchCount)
	require.Equal(t, 5*time.Second, cfg.PublishTimeout)
}

func TestNormalizeRejectsInvalidURL(t *testing.T) {
	_, err := Config{URL: "127.0.0.1:5672"}.Normalize()

	require.Error(t, err)
}

func TestNormalizeKeepsCallerValues(t *testing.T) {
	cfg, err := Config{
		URL:            "amqp://bw:bw123456@127.0.0.1:5672/",
		Exchange:       "test.events",
		ExchangeType:   "direct",
		Queue:          "test.queue",
		RoutingKeys:    []string{"test.key"},
		PrefetchCount:  1,
		PublishTimeout: time.Second,
		ConsumeTimeout: 2 * time.Second,
		ReconnectDelay: 100 * time.Millisecond,
	}.Normalize()

	require.NoError(t, err)
	require.Equal(t, "amqp://bw:bw123456@127.0.0.1:5672/", cfg.URL)
	require.Equal(t, "test.events", cfg.Exchange)
	require.Equal(t, "test.queue", cfg.Queue)
	require.Equal(t, []string{"test.key"}, cfg.RoutingKeys)
}

func TestPublishRequiresRoutingKey(t *testing.T) {
	c := &Client{cfg: DefaultConfig()}

	err := c.Publish(context.Background(), "", []byte("{}"))

	require.Error(t, err)
}
