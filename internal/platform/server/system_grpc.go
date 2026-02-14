package server

import (
	"context"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/clock"
)

type SystemService struct {
	rgsv1.UnimplementedSystemServiceServer

	StartedAt time.Time
	Clock     clock.Clock
	Version   string
}

func (s SystemService) GetSystemStatus(_ context.Context, req *rgsv1.GetSystemStatusRequest) (*rgsv1.GetSystemStatusResponse, error) {
	var requestID string
	if req != nil && req.Meta != nil {
		requestID = req.Meta.RequestId
	}

	now := s.Clock.Now().UTC()
	return &rgsv1.GetSystemStatusResponse{
		Meta: &rgsv1.ResponseMeta{
			RequestId:    requestID,
			ResultCode:   rgsv1.ResultCode_RESULT_CODE_OK,
			DenialReason: "",
			ServerTime:   now.Format(time.RFC3339Nano),
		},
		ServiceName: "open-rgs-go",
		Version:     s.Version,
		Uptime:      now.Sub(s.StartedAt).String(),
	}, nil
}
