// Package rabbitmqx 封装 RabbitMQ 的发布与订阅能力。
// 连接断开后会自动重连并重建交换机、队列声明，适合常驻服务使用。
package rabbitmqx

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Handler 处理一条消费到的消息体；返回错误时消息被拒绝且不重新入队。
type Handler func(ctx context.Context, body []byte) error

// Client 是 RabbitMQ 客户端，聚合一条 AMQP 连接与发布/订阅能力。
// 零值不可用，必须通过 New 创建。
type Client struct {
	cfg Config

	mu     sync.Mutex
	conn   *amqp.Connection
	ch     *amqp.Channel
	closed bool
}

// New 连接 RabbitMQ，打开信道并声明交换机。
func New(cfg Config) (*Client, error) {
	normalized, err := cfg.Normalize()
	if err != nil {
		return nil, err
	}
	c := &Client{cfg: normalized}
	if err := c.dial(); err != nil {
		return nil, err
	}
	go c.watchConnection()
	return c, nil
}

// dial 建立连接并完成初始信道与交换机声明。
func (c *Client) dial() error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}
	if err := ch.ExchangeDeclare(c.cfg.Exchange, c.cfg.ExchangeType, c.cfg.Durable, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = ch.Close()
		_ = conn.Close()
		return errors.New("rabbitmq client is closed")
	}
	c.conn = conn
	c.ch = ch
	go c.notifyClose(conn)
	return nil
}

// notifyClose 监听连接关闭事件，触发后台重连。
func (c *Client) notifyClose(conn *amqp.Connection) {
	closeErr := make(chan *amqp.Error, 1)
	conn.NotifyClose(closeErr)
	<-closeErr

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.ch = nil
	c.mu.Unlock()

	go c.watchConnection()
}

// watchConnection 在连接断开后按固定间隔重试，直到重连成功或客户端关闭。
func (c *Client) watchConnection() {
	for {
		time.Sleep(c.cfg.ReconnectDelay)
		c.mu.Lock()
		closed := c.closed
		hasConn := c.conn != nil
		c.mu.Unlock()
		if closed || hasConn {
			return
		}
		if err := c.dial(); err == nil {
			return
		}
	}
}

// channel 返回当前可用的发布信道；连接断开返回错误，调用方可等待重连后重试。
func (c *Client) channel() (*amqp.Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("rabbitmq client is closed")
	}
	if c.ch == nil {
		return nil, errors.New("rabbitmq channel is reconnecting")
	}
	return c.ch, nil
}

// Publish 向指定 routing key 发布消息体，消息为持久化消息。
func (c *Client) Publish(ctx context.Context, routingKey string, body []byte) error {
	if routingKey == "" {
		return errors.New("routing key is required")
	}
	publishCtx, cancel := context.WithTimeout(ctx, c.cfg.PublishTimeout)
	defer cancel()

	for {
		ch, err := c.channel()
		if err != nil {
			select {
			case <-publishCtx.Done():
				return publishCtx.Err()
			case <-time.After(c.cfg.ReconnectDelay):
				continue
			}
		}
		err = ch.PublishWithContext(publishCtx, c.cfg.Exchange, routingKey, false, false, amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Timestamp:    time.Now(),
			Body:         body,
		})
		if err == nil {
			return nil
		}
		if publishCtx.Err() != nil {
			return publishCtx.Err()
		}
		select {
		case <-publishCtx.Done():
			return publishCtx.Err()
		case <-time.After(c.cfg.ReconnectDelay):
		}
	}
}

// DeclareQueue 声明队列并按 routing keys 绑定到已配置的交换机，可重复调用。
func (c *Client) DeclareQueue(queue string, routingKeys []string) error {
	if queue == "" {
		return errors.New("queue name is required")
	}
	ch, err := c.channel()
	if err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(queue, c.cfg.Durable, false, false, false, nil); err != nil {
		return err
	}
	for _, key := range routingKeys {
		if key == "" {
			continue
		}
		if err := ch.QueueBind(queue, key, c.cfg.Exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

// Consume 订阅队列并调用 handler 处理每条消息，阻塞直到 ctx 取消。
// 连接断开重连后会自动重新声明队列并恢复消费。
func (c *Client) Consume(ctx context.Context, queue string, routingKeys []string, handler Handler) error {
	if handler == nil {
		return errors.New("handler is required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.DeclareQueue(queue, routingKeys); err != nil {
			if !c.waitReconnect(ctx) {
				return ctx.Err()
			}
			continue
		}
		ch, err := c.channel()
		if err != nil {
			if !c.waitReconnect(ctx) {
				return ctx.Err()
			}
			continue
		}
		if err := ch.Qos(c.cfg.PrefetchCount, 0, false); err != nil {
			if !c.waitReconnect(ctx) {
				return ctx.Err()
			}
			continue
		}
		deliveries, err := ch.Consume(queue, c.cfg.ConsumerTag, false, false, false, false, nil)
		if err != nil {
			if !c.waitReconnect(ctx) {
				return ctx.Err()
			}
			continue
		}
		if err := c.dispatch(ctx, deliveries, handler); err != nil {
			return err
		}
	}
}

// dispatch 循环处理投递，deliveries 关闭（连接断开）时返回 nil 由上层重连。
func (c *Client) dispatch(ctx context.Context, deliveries <-chan amqp.Delivery, handler Handler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				// 连接断开，等待重连后重新消费。
				return nil
			}
			hctx, cancel := context.WithTimeout(ctx, c.cfg.ConsumeTimeout)
			err := handler(hctx, delivery.Body)
			cancel()
			if err != nil {
				_ = delivery.Nack(false, false)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

// waitReconnect 等待一个重连周期；ctx 取消时返回 false。
func (c *Client) waitReconnect(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(c.cfg.ReconnectDelay):
		return true
	}
}

// Close 关闭客户端并释放底层连接；重复调用是安全的。
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var errs []error
	if c.ch != nil {
		if err := c.ch.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// String 返回客户端的人类可读描述，便于日志排查。
func (c *Client) String() string {
	return fmt.Sprintf("rabbitmq(exchange=%s, queue=%s)", c.cfg.Exchange, c.cfg.Queue)
}
