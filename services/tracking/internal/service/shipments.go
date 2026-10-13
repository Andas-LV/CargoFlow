package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	shipmentv1 "github.com/Andas-LV/cargoflow/gen/go/shipment/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
)

// GRPCShipments checks shipment ownership by calling the shipment service.
// This is the only place where tracking reads another service's data: over gRPC, never via its DB.
type GRPCShipments struct {
	client shipmentv1.ShipmentServiceClient
}

func NewGRPCShipments(client shipmentv1.ShipmentServiceClient) *GRPCShipments {
	return &GRPCShipments{client: client}
}

// CheckOwner returns ErrNotFound if the shipment does not exist or belongs to someone else.
func (g *GRPCShipments) CheckOwner(ctx context.Context, userID, shipmentID string) error {
	// Forward the caller identity: the shipment service enforces ownership itself.
	_, err := g.client.GetShipment(grpcx.WithUserID(ctx, userID), &shipmentv1.GetShipmentRequest{Id: shipmentID})
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.NotFound, codes.InvalidArgument:
		return ErrNotFound
	default:
		return fmt.Errorf("shipment service: %w", err)
	}
}
