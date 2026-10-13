// Package repository stores shipment positions in MongoDB.
package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const positionsCollection = "positions"

// Position is one point of a shipment's route.
type Position struct {
	ID         string    `bson:"_id"`
	ShipmentID string    `bson:"shipment_id"`
	Station    string    `bson:"station"`
	Lat        float64   `bson:"lat"`
	Lon        float64   `bson:"lon"`
	Source     string    `bson:"source"`
	RecordedAt time.Time `bson:"recorded_at"`
}

type Positions struct {
	coll *mongo.Collection
}

func NewPositions(db *mongo.Database) *Positions {
	return &Positions{coll: db.Collection(positionsCollection)}
}

// EnsureIndexes creates the route lookup index; idempotent.
func (r *Positions) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "shipment_id", Value: 1}, {Key: "recorded_at", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("create positions index: %w", err)
	}
	return nil
}

// Insert stores p. It returns false without error when a document with the same _id already
// exists, which makes event handlers that use the event ID as _id idempotent.
func (r *Positions) Insert(ctx context.Context, p Position) (bool, error) {
	_, err := r.coll.InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListByShipment returns the route in chronological order.
func (r *Positions) ListByShipment(ctx context.Context, shipmentID string) ([]Position, error) {
	cur, err := r.coll.Find(ctx,
		bson.D{{Key: "shipment_id", Value: shipmentID}},
		options.Find().SetSort(bson.D{{Key: "recorded_at", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	positions := []Position{}
	if err := cur.All(ctx, &positions); err != nil {
		return nil, err
	}
	return positions, nil
}
