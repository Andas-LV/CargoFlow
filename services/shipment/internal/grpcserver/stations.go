package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	shipmentv1 "github.com/Andas-LV/cargoflow/gen/go/shipment/v1"
	"github.com/Andas-LV/cargoflow/services/shipment/internal/stations"
)

// StationServer exposes the station directory. It is reference data, so it does not require
// a user identity: other services (tracking) call it directly.
type StationServer struct {
	shipmentv1.UnimplementedStationServiceServer
	dir *stations.Directory
}

func NewStationServer(dir *stations.Directory) *StationServer {
	return &StationServer{dir: dir}
}

func (s *StationServer) SearchStations(_ context.Context, req *shipmentv1.SearchStationsRequest) (*shipmentv1.SearchStationsResponse, error) {
	if strings.TrimSpace(req.GetQuery()) == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	found := s.dir.Search(req.GetQuery(), int(req.GetPageSize()))
	resp := &shipmentv1.SearchStationsResponse{Stations: make([]*shipmentv1.Station, 0, len(found))}
	for _, st := range found {
		resp.Stations = append(resp.Stations, stationToProto(st))
	}
	return resp, nil
}

func (s *StationServer) GetStation(_ context.Context, req *shipmentv1.GetStationRequest) (*shipmentv1.GetStationResponse, error) {
	st, ok := s.dir.Get(req.GetId())
	if !ok {
		return nil, status.Error(codes.NotFound, "station not found")
	}
	return &shipmentv1.GetStationResponse{Station: stationToProto(st)}, nil
}

var stationKindToProto = map[string]shipmentv1.StationKind{
	stations.KindStation: shipmentv1.StationKind_STATION_KIND_STATION,
	stations.KindHalt:    shipmentv1.StationKind_STATION_KIND_HALT,
}

func stationToProto(st stations.Station) *shipmentv1.Station {
	return &shipmentv1.Station{
		Id:     st.ID,
		Esr:    st.Esr,
		Kind:   stationKindToProto[st.Kind],
		Name:   st.Name,
		NameRu: st.NameRu,
		NameKk: st.NameKk,
		NameEn: st.NameEn,
		Lat:    st.Lat,
		Lon:    st.Lon,
	}
}
