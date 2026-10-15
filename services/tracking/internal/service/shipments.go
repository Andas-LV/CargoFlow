package service

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	shipmentv1 "github.com/Andas-LV/cargoflow/gen/go/shipment/v1"
	"github.com/Andas-LV/cargoflow/pkg/grpcx"
)

// GRPCShipments reads shipment-owned data over gRPC: shipment ownership and the station
// directory. This is the only way tracking sees another service's data, never via its DB.
type GRPCShipments struct {
	shipments shipmentv1.ShipmentServiceClient
	stations  shipmentv1.StationServiceClient
}

func NewGRPCShipments(shipments shipmentv1.ShipmentServiceClient, stations shipmentv1.StationServiceClient) *GRPCShipments {
	return &GRPCShipments{shipments: shipments, stations: stations}
}

// CheckOwner returns ErrNotFound if the shipment does not exist or belongs to someone else.
func (g *GRPCShipments) CheckOwner(ctx context.Context, userID, shipmentID string) error {
	// Forward the caller identity: the shipment service enforces ownership itself.
	_, err := g.shipments.GetShipment(grpcx.WithUserID(ctx, userID), &shipmentv1.GetShipmentRequest{Id: shipmentID})
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.NotFound, codes.InvalidArgument:
		return ErrNotFound
	default:
		return fmt.Errorf("shipment service: %w", err)
	}
}

// Station returns a directory entry; ErrInvalidInput when the station does not exist.
func (g *GRPCShipments) Station(ctx context.Context, id string) (Station, error) {
	resp, err := g.stations.GetStation(ctx, &shipmentv1.GetStationRequest{Id: id})
	switch status.Code(err) {
	case codes.OK:
	case codes.NotFound:
		return Station{}, fmt.Errorf("%w: unknown station %q", ErrInvalidInput, id)
	default:
		return Station{}, fmt.Errorf("station service: %w", err)
	}
	st := resp.GetStation()
	return Station{ID: st.GetId(), Name: st.GetNameRu(), Lat: st.GetLat(), Lon: st.GetLon()}, nil
}
