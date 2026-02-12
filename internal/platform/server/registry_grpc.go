package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"google.golang.org/protobuf/proto"
)

type RegistryService struct {
	rgsv1.UnimplementedRegistryServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu                   sync.Mutex
	equipment            map[string]*rgsv1.Equipment
	nextAuditID          int64
	db                   *sql.DB
	disableInMemoryCache bool
}

func NewRegistryService(clk clock.Clock, db ...*sql.DB) *RegistryService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &RegistryService{
		Clock:      clk,
		AuditStore: audit.NewInMemoryStore(),
		equipment:  make(map[string]*rgsv1.Equipment),
		db:         handle,
	}
}

func (s *RegistryService) SetDisableInMemoryCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemoryCache = disable
}

func (s *RegistryService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *RegistryService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *RegistryService) authorize(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
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

func equipmentSnapshot(eq *rgsv1.Equipment) []byte {
	if eq == nil {
		return []byte(`{}`)
	}
	b, _ := json.Marshal(eq)
	return b
}

func (s *RegistryService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "registry-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *RegistryService) appendAudit(meta *rgsv1.RequestMeta, objectID, action string, before, after []byte, result audit.Result, reason string) error {
	if s.AuditStore == nil {
		return audit.ErrCorruptChain
	}
	actorID := "system"
	actorType := "service"
	if meta != nil && meta.Actor != nil {
		actorID = meta.Actor.ActorId
		actorType = meta.Actor.ActorType.String()
	}
	now := s.now()
	_, err := s.AuditStore.Append(audit.Event{
		AuditID:      s.nextAuditIDLocked(),
		OccurredAt:   now,
		RecordedAt:   now,
		ActorID:      actorID,
		ActorType:    actorType,
		ObjectType:   "equipment",
		ObjectID:     objectID,
		Action:       action,
		Before:       before,
		After:        after,
		Result:       result,
		Reason:       reason,
		PartitionDay: now.Format("2006-01-02"),
	})
	return err
}

func cloneEquipment(eq *rgsv1.Equipment) *rgsv1.Equipment {
	if eq == nil {
		return nil
	}
	cp, _ := proto.Clone(eq).(*rgsv1.Equipment)
	return cp
}

func (s *RegistryService) UpsertEquipment(ctx context.Context, req *rgsv1.UpsertEquipmentRequest) (*rgsv1.UpsertEquipmentResponse, error) {
	if req == nil || req.Equipment == nil || req.Equipment.EquipmentId == "" {
		return &rgsv1.UpsertEquipmentResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "equipment.equipment_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, req.Equipment.EquipmentId, "upsert_equipment", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.UpsertEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing := cloneEquipment(s.equipment[req.Equipment.EquipmentId])
	if s.db != nil {
		var err error
		existing, err = s.getEquipmentFromDB(ctx, req.Equipment.EquipmentId)
		if err != nil {
			return &rgsv1.UpsertEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	}
	before := equipmentSnapshot(existing)

	now := s.now().Format(time.RFC3339Nano)
	upsert := cloneEquipment(req.Equipment)
	if upsert.CreatedAt == "" {
		if existing != nil && existing.CreatedAt != "" {
			upsert.CreatedAt = existing.CreatedAt
		} else {
			upsert.CreatedAt = now
		}
	}
	upsert.UpdatedAt = now

	after := equipmentSnapshot(upsert)
	if err := s.appendAudit(req.Meta, upsert.EquipmentId, "upsert_equipment", before, after, audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.UpsertEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}

	if s.db != nil {
		if err := s.upsertEquipmentInDB(ctx, upsert); err != nil {
			return &rgsv1.UpsertEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	} else {
		if !s.disableInMemoryCache {
			s.equipment[upsert.EquipmentId] = upsert
		}
	}

	return &rgsv1.UpsertEquipmentResponse{
		Meta:      s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Equipment: cloneEquipment(upsert),
	}, nil
}

func (s *RegistryService) GetEquipment(ctx context.Context, req *rgsv1.GetEquipmentRequest) (*rgsv1.GetEquipmentResponse, error) {
	if req == nil || req.EquipmentId == "" {
		return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "equipment_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, req.EquipmentId, "get_equipment", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	if s.db != nil {
		eq, err := s.getEquipmentFromDB(ctx, req.EquipmentId)
		if err != nil {
			return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if eq == nil {
			return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "equipment not found")}, nil
		} else {
			return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Equipment: eq}, nil
		}
	}
	if s.disableInMemoryCache {
		return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "equipment not found")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	eq := cloneEquipment(s.equipment[req.EquipmentId])
	if eq == nil {
		return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "equipment not found")}, nil
	}
	return &rgsv1.GetEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Equipment: eq}, nil
}

func (s *RegistryService) ListEquipment(ctx context.Context, req *rgsv1.ListEquipmentRequest) (*rgsv1.ListEquipmentResponse, error) {
	if req == nil {
		req = &rgsv1.ListEquipmentRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "", "list_equipment", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ListEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	start := 0
	if req.PageToken != "" {
		if parsed, err := strconv.Atoi(req.PageToken); err == nil && parsed >= 0 {
			start = parsed
		}
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	if s.db != nil {
		items, err := s.listEquipmentFromDB(ctx, req.StatusFilter, pageSize, start)
		if err != nil {
			return &rgsv1.ListEquipmentResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		next := ""
		if len(items) == pageSize {
			next = strconv.Itoa(start + len(items))
		}
		return &rgsv1.ListEquipmentResponse{
			Meta:          s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
			Equipment:     items,
			NextPageToken: next,
		}, nil
	}
	if s.disableInMemoryCache {
		return &rgsv1.ListEquipmentResponse{
			Meta:          s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
			Equipment:     nil,
			NextPageToken: "",
		}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.equipment))
	for id := range s.equipment {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	filtered := make([]*rgsv1.Equipment, 0, len(ids))
	for _, id := range ids {
		eq := s.equipment[id]
		if req.StatusFilter != rgsv1.EquipmentStatus_EQUIPMENT_STATUS_UNSPECIFIED && eq.Status != req.StatusFilter {
			continue
		}
		filtered = append(filtered, cloneEquipment(eq))
	}

	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}

	next := ""
	if end < len(filtered) {
		next = strconv.Itoa(end)
	}

	return &rgsv1.ListEquipmentResponse{
		Meta:          s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Equipment:     filtered[start:end],
		NextPageToken: next,
	}, nil
}
