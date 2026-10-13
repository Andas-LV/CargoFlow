// Package natsx wraps NATS JetStream: connection, the CARGO stream, publishing protobuf
// events and consuming them with OpenTelemetry context propagated through message headers.
package natsx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Stream and subjects of the CARGO event stream. Payloads are defined in proto/events/v1.
const (
	StreamName = "CARGO"

	SubjectShipmentCreated       = "shipment.created"
	SubjectShipmentStatusChanged = "shipment.status_changed"
	SubjectPositionUpdated       = "tracking.position_updated"
)

// Config is embedded into configs of services that use NATS.
type Config struct {
	URL string `env:"NATS_URL,required"`
	// CredsFile is a path to a .creds file (Synadia Cloud). Empty means no auth.
	CredsFile string `env:"NATS_CREDS_FILE"`
}

// Conn bundles the core connection and its JetStream context.
type Conn struct {
	NC *nats.Conn
	JS jetstream.JetStream
}

// Connect connects to NATS with infinite reconnects and creates a JetStream context.
func Connect(cfg Config, name string, log *slog.Logger) (*Conn, error) {
	opts := []nats.Option{
		nats.Name(name),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warn("nats disconnected", "err", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info("nats reconnected", "url", nc.ConnectedUrlRedacted())
		}),
	}
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	return &Conn{NC: nc, JS: js}, nil
}

// EnsureStream creates or updates the CARGO stream. Safe to call from every service on start.
func (c *Conn) EnsureStream(ctx context.Context) error {
	_, err := c.JS.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       StreamName,
		Subjects:   []string{"shipment.>", "tracking.>"},
		Storage:    jetstream.FileStorage,
		MaxAge:     7 * 24 * time.Hour,
		Duplicates: 2 * time.Minute, // window for Nats-Msg-Id deduplication
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", StreamName, err)
	}
	return nil
}

// Check is a readiness check for the connection.
func (c *Conn) Check(context.Context) error {
	if s := c.NC.Status(); s != nats.CONNECTED {
		return fmt.Errorf("nats status %s", s)
	}
	return nil
}

// Close drains the connection: in-flight messages are processed before closing.
func (c *Conn) Close() {
	_ = c.NC.Drain()
}
