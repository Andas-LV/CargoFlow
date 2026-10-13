// Package mongox connects to MongoDB.
package mongox

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Connect opens a client for uri (mongodb:// or mongodb+srv://) and pings the primary.
func Connect(ctx context.Context, uri string) (*mongo.Client, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client, nil
}

// Check returns a readiness check for client.
func Check(client *mongo.Client) func(ctx context.Context) error {
	return func(ctx context.Context) error { return client.Ping(ctx, readpref.Primary()) }
}
