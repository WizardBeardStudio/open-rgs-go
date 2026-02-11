package server

import (
	"context"
	"sort"
	"strconv"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
)

type AuditService struct {
	rgsv1.UnimplementedAuditServiceServer

	Clock clock.Clock

	stores      []*audit.InMemoryStore
	remoteGuard *RemoteAccessGuard
}

func NewAuditService(clk clock.Clock, remoteGuard *RemoteAccessGuard, stores ...*audit.InMemoryStore) *AuditService {
	return &AuditService{Clock: clk, remoteGuard: remoteGuard, stores: stores}
}

func (s *AuditService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *AuditService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *AuditService) authorize(meta *rgsv1.RequestMeta) (bool, string) {
	if meta == nil || meta.Actor == nil {
		return false, "actor is required"
	}
	if meta.Actor.ActorId == "" || meta.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return false, "actor binding is required"
	}
	switch meta.Actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func paginate[T any](items []T, pageToken string, pageSize int32) ([]T, string) {
	start := 0
	if pageToken != "" {
		if p, err := strconv.Atoi(pageToken); err == nil && p >= 0 {
			start = p
		}
	}
	if start > len(items) {
		start = len(items)
	}
	sz := int(pageSize)
	if sz <= 0 {
		sz = 100
	}
	end := start + sz
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return items[start:end], next
}

func (s *AuditService) ListAuditEvents(_ context.Context, req *rgsv1.ListAuditEventsRequest) (*rgsv1.ListAuditEventsResponse, error) {
	if req == nil {
		req = &rgsv1.ListAuditEventsRequest{}
	}
	if ok, reason := s.authorize(req.Meta); !ok {
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	events := make([]*rgsv1.AuditEventRecord, 0)
	for _, st := range s.stores {
		if st == nil {
			continue
		}
		for _, e := range st.Events() {
			if req.ObjectTypeFilter != "" && e.ObjectType != req.ObjectTypeFilter {
				continue
			}
			events = append(events, &rgsv1.AuditEventRecord{
				AuditId:    e.AuditID,
				OccurredAt: e.OccurredAt.Format(time.RFC3339Nano),
				RecordedAt: e.RecordedAt.Format(time.RFC3339Nano),
				ActorId:    e.ActorID,
				ActorType:  e.ActorType,
				ObjectType: e.ObjectType,
				ObjectId:   e.ObjectID,
				Action:     e.Action,
				Result:     string(e.Result),
				Reason:     e.Reason,
			})
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].RecordedAt == events[j].RecordedAt {
			return events[i].AuditId < events[j].AuditId
		}
		return events[i].RecordedAt > events[j].RecordedAt
	})

	page, next := paginate(events, req.PageToken, req.PageSize)
	return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: page, NextPageToken: next}, nil
}

func (s *AuditService) ListRemoteAccessActivities(_ context.Context, req *rgsv1.ListRemoteAccessActivitiesRequest) (*rgsv1.ListRemoteAccessActivitiesResponse, error) {
	if req == nil {
		req = &rgsv1.ListRemoteAccessActivitiesRequest{}
	}
	if ok, reason := s.authorize(req.Meta); !ok {
		return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	activities := make([]*rgsv1.RemoteAccessActivityRecord, 0)
	if s.remoteGuard != nil {
		for _, a := range s.remoteGuard.Activities() {
			activities = append(activities, &rgsv1.RemoteAccessActivityRecord{
				Timestamp:       a.Timestamp,
				SourceIp:        a.SourceIP,
				SourcePort:      a.SourcePort,
				Destination:     a.Destination,
				DestinationPort: a.DestinationPort,
				Path:            a.Path,
				Method:          a.Method,
				Allowed:         a.Allowed,
				Reason:          a.Reason,
			})
		}
	}

	sort.SliceStable(activities, func(i, j int) bool {
		if activities[i].Timestamp == activities[j].Timestamp {
			return activities[i].Path < activities[j].Path
		}
		return activities[i].Timestamp > activities[j].Timestamp
	})

	page, next := paginate(activities, req.PageToken, req.PageSize)
	return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Activities: page, NextPageToken: next}, nil
}
