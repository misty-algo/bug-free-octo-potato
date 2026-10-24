package rabbitmqx

import (
	"fmt"
	"strings"
	"time"
)

// Config 控制 RabbitMQ 连接、交换机与队列的创建。
// URL 使用 AMQP 协议格式，例如 amqp://user:pass@127.0.0.1:5672/。
type Config struct {
	URL            string        `mapstructure:"url" yaml:"url"`
	Exchange       string        `mapstructure:"exchange" yaml:"exchange"`
	ExchangeType   string        `mapstructure:"exchange_type" yaml:"exchange_type"`
	Queue          string        `mapstructure:"queue" yaml:"queue"`
	RoutingKeys    []string      `mapstructure:"routing_keys" yaml:"routing_keys"`
	ConsumerTag    string        `mapstructure:"consumer_tag" yaml:"consumer_tag"`
	PrefetchCount  int           `mapstructure:"prefetch_count" yaml:"prefetch_count"`
	Durable        bool          `mapstructure:"durable" yaml:"durable"`
	PublishTimeout time.Duration `mapstructure:"publish_timeout" yaml:"publish_timeout"`
	ConsumeTimeout time.Duration `mapstructure:"consume_timeout" yaml:"consume_timeout"`
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay" yaml:"reconnect_delay"`
}

// DefaultConfig 返回本地开发环境可直接使用的 RabbitMQ 配置。
// 生产环境应通过本地 YAML 或 Nacos 配置覆盖 URL、交换机与队列参数。
func DefaultConfig() Config {
	return Config{
		URL:            "amqp://guest:guest@127.0.0.1:5672/",
		Exchange:       "xiaolanshu.events",
		ExchangeType:   "topic",
		Queue:          "xiaolanshu.order.created",
		RoutingKeys:    []string{"order.created"},
		ConsumerTag:    "xiaolanshu-consumer",
		PrefetchCount:  10,
		Durable:        true,
		PublishTimeout: 5 * time.Second,
		ConsumeTimeout: 30 * time.Second,
		ReconnectDelay: 3 * time.Second,
	}
}

// Normalize 在保留调用方配置的同时补齐默认值，并校验必填项。
func (cfg Config) Normalize() (Config, error) {
	defaults := DefaultConfig()
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		cfg.URL = defaults.URL
	}
	if cfg.Exchange == "" {
		cfg.Exchange = defaults.Exchange
	}
	if cfg.ExchangeType == "" {
		cfg.ExchangeType = defaults.ExchangeType
	}
	if cfg.Queue == "" {
		cfg.Queue = defaults.Queue
	}
	if len(cfg.RoutingKeys) == 0 {
		cfg.RoutingKeys = defaults.RoutingKeys
	}
	if cfg.ConsumerTag == "" {
		cfg.ConsumerTag = defaults.ConsumerTag
	}
	if cfg.PrefetchCount <= 0 {
		cfg.PrefetchCount = defaults.PrefetchCount
	}
	if cfg.PublishTimeout <= 0 {
		cfg.PublishTimeout = defaults.PublishTimeout
	}
	if cfg.ConsumeTimeout <= 0 {
		cfg.ConsumeTimeout = defaults.ConsumeTimeout
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = defaults.ReconnectDelay
	}
	if !strings.HasPrefix(cfg.URL, "amqp://") && !strings.HasPrefix(cfg.URL, "amqps://") {
		return Config{}, fmt.Errorf("rabbitmq url %q must start with amqp:// or amqps://", cfg.URL)
	}
	return cfg, nil
}
