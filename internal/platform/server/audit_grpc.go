package server

import (
	"context"
	"database/sql"
	"fmt"
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
	db          *sql.DB
}

const maxAuditPageSize = 1000

func NewAuditService(clk clock.Clock, remoteGuard *RemoteAccessGuard, stores ...*audit.InMemoryStore) *AuditService {
	return &AuditService{Clock: clk, remoteGuard: remoteGuard, stores: stores}
}

func (s *AuditService) SetDB(db *sql.DB) {
	if s == nil {
		return
	}
	s.db = db
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

func (s *AuditService) authorize(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	switch actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func paginate[T any](items []T, pageToken string, pageSize int32) ([]T, string, error) {
	start := 0
	if pageToken != "" {
		p, err := strconv.Atoi(pageToken)
		if err != nil || p < 0 {
			return nil, "", fmt.Errorf("invalid page token")
		}
		start = p
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
	return items[start:end], next, nil
}

func validatePageToken(pageToken string) error {
	if pageToken == "" {
		return nil
	}
	p, err := strconv.Atoi(pageToken)
	if err != nil || p < 0 {
		return fmt.Errorf("invalid page token")
	}
	return nil
}

func (s *AuditService) ListAuditEvents(ctx context.Context, req *rgsv1.ListAuditEventsRequest) (*rgsv1.ListAuditEventsResponse, error) {
	if req == nil {
		req = &rgsv1.ListAuditEventsRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if req.PageSize < 0 {
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "page_size must be non-negative")}, nil
	}
	if err := validatePageToken(req.PageToken); err != nil {
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_token")}, nil
	}
	if req.PageSize > maxAuditPageSize {
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "page_size exceeds max allowed")}, nil
	}
	if s.db != nil {
		rows, next, err := listAuditEventsFromDB(ctx, s.db, req.ObjectTypeFilter, req.PageToken, req.PageSize)
		if err != nil {
			return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: rows, NextPageToken: next}, nil
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

	page, next, err := paginate(events, req.PageToken, req.PageSize)
	if err != nil {
		return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_token")}, nil
	}
	return &rgsv1.ListAuditEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: page, NextPageToken: next}, nil
}

func (s *AuditService) ListRemoteAccessActivities(ctx context.Context, req *rgsv1.ListRemoteAccessActivitiesRequest) (*rgsv1.ListRemoteAccessActivitiesResponse, error) {
	if req == nil {
		req = &rgsv1.ListRemoteAccessActivitiesRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if req.PageSize < 0 {
		return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "page_size must be non-negative")}, nil
	}
	if err := validatePageToken(req.PageToken); err != nil {
		return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_token")}, nil
	}
	if req.PageSize > maxAuditPageSize {
		return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "page_size exceeds max allowed")}, nil
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

	page, next, err := paginate(activities, req.PageToken, req.PageSize)
	if err != nil {
		return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_token")}, nil
	}
	return &rgsv1.ListRemoteAccessActivitiesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Activities: page, NextPageToken: next}, nil
}

func (s *AuditService) VerifyAuditChain(ctx context.Context, req *rgsv1.VerifyAuditChainRequest) (*rgsv1.VerifyAuditChainResponse, error) {
	if req == nil {
		req = &rgsv1.VerifyAuditChainRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		return &rgsv1.VerifyAuditChainResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason), Valid: false}, nil
	}
	if req.PartitionDay != "" {
		if _, err := time.Parse("2006-01-02", req.PartitionDay); err != nil {
			return &rgsv1.VerifyAuditChainResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "partition_day must be YYYY-MM-DD"), Valid: false}, nil
		}
	}
	if s.db == nil {
		return &rgsv1.VerifyAuditChainResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable"), Valid: false}, nil
	}
	if err := verifyAuditChainFromDB(ctx, s.db, req.PartitionDay); err != nil {
		return &rgsv1.VerifyAuditChainResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit chain verification failed"), Valid: false}, nil
	}
	return &rgsv1.VerifyAuditChainResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Valid: true}, nil
}
