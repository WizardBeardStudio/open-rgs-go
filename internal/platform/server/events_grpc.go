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

type bufferStatus string

const (
	bufferQueued       bufferStatus = "queued"
	bufferAcknowledged bufferStatus = "acknowledged"
)

type ingestionBufferRecord struct {
	bufferID       string
	recordKind     string
	equipmentID    string
	sourceRecordID string
	status         bufferStatus
	occurredAt     string
	receivedAt     string
	recordedAt     string
}

type EventsService struct {
	rgsv1.UnimplementedEventsServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu sync.Mutex

	events               map[string]*rgsv1.SignificantEvent
	meters               map[string]*rgsv1.MeterRecord
	eventOrder           []string
	meterOrder           []string
	buffers              []ingestionBufferRecord
	bufferCap            int
	disabled             bool
	nextAuditID          int64
	nextBuffer           int64
	db                   *sql.DB
	disableInMemoryCache bool
}

func NewEventsService(clk clock.Clock, db ...*sql.DB) *EventsService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &EventsService{
		Clock:      clk,
		AuditStore: audit.NewInMemoryStore(),
		events:     make(map[string]*rgsv1.SignificantEvent),
		meters:     make(map[string]*rgsv1.MeterRecord),
		bufferCap:  1024,
		db:         handle,
	}
}

func (s *EventsService) SetDisableInMemoryCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemoryCache = disable
}

func (s *EventsService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *EventsService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *EventsService) authorizeWrite(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
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

func (s *EventsService) authorizeRead(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
	return s.authorizeWrite(ctx, meta)
}

func (s *EventsService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "events-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *EventsService) nextBufferIDLocked() string {
	s.nextBuffer++
	return "buf-" + strconv.FormatInt(s.nextBuffer, 10)
}

func (s *EventsService) appendAudit(meta *rgsv1.RequestMeta, objectType, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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
		ObjectType:   objectType,
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

func cloneEvent(in *rgsv1.SignificantEvent) *rgsv1.SignificantEvent {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.SignificantEvent)
	return cp
}

func cloneMeter(in *rgsv1.MeterRecord) *rgsv1.MeterRecord {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.MeterRecord)
	return cp
}

func (s *EventsService) submitBlocked(meta *rgsv1.RequestMeta, objectType, objectID, action, reason string) {
	_ = s.appendAudit(meta, objectType, objectID, action, []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
}

func (s *EventsService) queueBufferLocked(kind, equipmentID, sourceRecordID, occurredAt string) (ingestionBufferRecord, bool) {
	if s.disabled {
		return ingestionBufferRecord{}, false
	}
	queued := 0
	for _, b := range s.buffers {
		if b.status == bufferQueued {
			queued++
		}
	}
	if queued >= s.bufferCap {
		s.disabled = true
		return ingestionBufferRecord{}, false
	}
	now := s.now().Format(time.RFC3339Nano)
	record := ingestionBufferRecord{
		bufferID:       s.nextBufferIDLocked(),
		recordKind:     kind,
		equipmentID:    equipmentID,
		sourceRecordID: sourceRecordID,
		status:         bufferQueued,
		occurredAt:     occurredAt,
		receivedAt:     now,
		recordedAt:     now,
	}
	s.buffers = append(s.buffers, record)
	return record, true
}

func (s *EventsService) acknowledgeBufferLocked(bufferID string) {
	for i := range s.buffers {
		if s.buffers[i].bufferID == bufferID {
			s.buffers[i].status = bufferAcknowledged
			return
		}
	}
}

func (s *EventsService) SubmitSignificantEvent(ctx context.Context, req *rgsv1.SubmitSignificantEventRequest) (*rgsv1.SubmitSignificantEventResponse, error) {
	if req == nil || req.Event == nil || req.Event.EventId == "" || req.Event.EquipmentId == "" {
		return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "event_id and equipment_id are required")}, nil
	}
	if ok, reason := s.authorizeWrite(ctx, req.Meta); !ok {
		s.submitBlocked(req.Meta, "significant_event", req.Event.EventId, "submit_significant_event", reason)
		return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.events[req.Event.EventId]; ok {
		return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Event: cloneEvent(s.events[req.Event.EventId])}, nil
	}

	buffer, ok := s.queueBufferLocked("significant_event", req.Event.EquipmentId, req.Event.EventId, req.Event.OccurredAt)
	if !ok {
		s.submitBlocked(req.Meta, "significant_event", req.Event.EventId, "submit_significant_event", "ingestion buffer exhausted")
		return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "ingestion buffer exhausted")}, nil
	}

	now := s.now().Format(time.RFC3339Nano)
	e := cloneEvent(req.Event)
	if e.OccurredAt == "" {
		e.OccurredAt = now
	}
	e.ReceivedAt = now
	e.RecordedAt = now

	before := []byte(`{}`)
	after, _ := json.Marshal(e)
	if err := s.appendAudit(req.Meta, "significant_event", e.EventId, "submit_significant_event", before, after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistSignificantEvent(ctx, req.Meta, e, buffer); err != nil {
		return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	if !s.disableInMemoryCache {
		s.events[e.EventId] = e
		s.eventOrder = append(s.eventOrder, e.EventId)
	}
	s.acknowledgeBufferLocked(buffer.bufferID)

	return &rgsv1.SubmitSignificantEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Event: cloneEvent(e)}, nil
}

func (s *EventsService) SubmitMeterSnapshot(ctx context.Context, req *rgsv1.SubmitMeterSnapshotRequest) (*rgsv1.SubmitMeterSnapshotResponse, error) {
	return s.submitMeter(ctx, req.Meta, req.Meter, rgsv1.MeterRecordType_METER_RECORD_TYPE_SNAPSHOT)
}

func (s *EventsService) SubmitMeterDelta(ctx context.Context, req *rgsv1.SubmitMeterDeltaRequest) (*rgsv1.SubmitMeterDeltaResponse, error) {
	resp, err := s.submitMeter(ctx, req.Meta, req.Meter, rgsv1.MeterRecordType_METER_RECORD_TYPE_DELTA)
	if err != nil {
		return nil, err
	}
	return &rgsv1.SubmitMeterDeltaResponse{Meta: resp.Meta, Meter: resp.Meter}, nil
}

func (s *EventsService) submitMeter(ctx context.Context, meta *rgsv1.RequestMeta, meter *rgsv1.MeterRecord, kind rgsv1.MeterRecordType) (*rgsv1.SubmitMeterSnapshotResponse, error) {
	if meter == nil || meter.MeterId == "" || meter.EquipmentId == "" || meter.MeterLabel == "" {
		return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "meter_id, equipment_id, and meter_label are required")}, nil
	}
	if ok, reason := s.authorizeWrite(ctx, meta); !ok {
		s.submitBlocked(meta, "meter_record", meter.MeterId, "submit_meter", reason)
		return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.meters[meter.MeterId]; ok {
		return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Meter: cloneMeter(existing)}, nil
	}

	buffer, ok := s.queueBufferLocked("meter", meter.EquipmentId, meter.MeterId, meter.OccurredAt)
	if !ok {
		s.submitBlocked(meta, "meter_record", meter.MeterId, "submit_meter", "ingestion buffer exhausted")
		return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "ingestion buffer exhausted")}, nil
	}

	now := s.now().Format(time.RFC3339Nano)
	m := cloneMeter(meter)
	if m.OccurredAt == "" {
		m.OccurredAt = now
	}
	m.RecordType = kind
	m.ReceivedAt = now
	m.RecordedAt = now

	before := []byte(`{}`)
	after, _ := json.Marshal(m)
	if err := s.appendAudit(meta, "meter_record", m.MeterId, "submit_meter", before, after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistMeterRecord(ctx, meta, m, buffer); err != nil {
		return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	if !s.disableInMemoryCache {
		s.meters[m.MeterId] = m
		s.meterOrder = append(s.meterOrder, m.MeterId)
	}
	s.acknowledgeBufferLocked(buffer.bufferID)

	return &rgsv1.SubmitMeterSnapshotResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Meter: cloneMeter(m)}, nil
}

func (s *EventsService) ListEvents(ctx context.Context, req *rgsv1.ListEventsRequest) (*rgsv1.ListEventsResponse, error) {
	if req == nil {
		req = &rgsv1.ListEventsRequest{}
	}
	if ok, reason := s.authorizeRead(ctx, req.Meta); !ok {
		s.submitBlocked(req.Meta, "significant_event", "", "list_events", reason)
		return &rgsv1.ListEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		start := 0
		if req.PageToken != "" {
			if p, err := strconv.Atoi(req.PageToken); err == nil && p >= 0 {
				start = p
			}
		}
		size := int(req.PageSize)
		if size <= 0 {
			size = 100
		}
		dbItems, err := s.listEventsFromDB(ctx, req.EquipmentId, size, start)
		if err != nil {
			return &rgsv1.ListEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		next := ""
		if len(dbItems) == size {
			next = strconv.Itoa(start + len(dbItems))
		}
		return &rgsv1.ListEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: dbItems, NextPageToken: next}, nil
	}
	if s.disableInMemoryCache {
		return &rgsv1.ListEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: nil, NextPageToken: ""}, nil
	}

	items := make([]*rgsv1.SignificantEvent, 0, len(s.eventOrder))
	for _, id := range s.eventOrder {
		e := s.events[id]
		if req.EquipmentId != "" && e.EquipmentId != req.EquipmentId {
			continue
		}
		items = append(items, cloneEvent(e))
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RecordedAt == items[j].RecordedAt {
			return items[i].EventId < items[j].EventId
		}
		return items[i].RecordedAt < items[j].RecordedAt
	})

	start := 0
	if req.PageToken != "" {
		if p, err := strconv.Atoi(req.PageToken); err == nil && p >= 0 {
			start = p
		}
	}
	if start > len(items) {
		start = len(items)
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 100
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}

	return &rgsv1.ListEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: items[start:end], NextPageToken: next}, nil
}

func (s *EventsService) ListMeters(ctx context.Context, req *rgsv1.ListMetersRequest) (*rgsv1.ListMetersResponse, error) {
	if req == nil {
		req = &rgsv1.ListMetersRequest{}
	}
	if ok, reason := s.authorizeRead(ctx, req.Meta); !ok {
		s.submitBlocked(req.Meta, "meter_record", "", "list_meters", reason)
		return &rgsv1.ListMetersResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		start := 0
		if req.PageToken != "" {
			if p, err := strconv.Atoi(req.PageToken); err == nil && p >= 0 {
				start = p
			}
		}
		size := int(req.PageSize)
		if size <= 0 {
			size = 100
		}
		dbItems, err := s.listMetersFromDB(ctx, req.EquipmentId, req.MeterLabel, size, start)
		if err != nil {
			return &rgsv1.ListMetersResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		next := ""
		if len(dbItems) == size {
			next = strconv.Itoa(start + len(dbItems))
		}
		return &rgsv1.ListMetersResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Meters: dbItems, NextPageToken: next}, nil
	}
	if s.disableInMemoryCache {
		return &rgsv1.ListMetersResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Meters: nil, NextPageToken: ""}, nil
	}

	items := make([]*rgsv1.MeterRecord, 0, len(s.meterOrder))
	for _, id := range s.meterOrder {
		m := s.meters[id]
		if req.EquipmentId != "" && m.EquipmentId != req.EquipmentId {
			continue
		}
		if req.MeterLabel != "" && m.MeterLabel != req.MeterLabel {
			continue
		}
		items = append(items, cloneMeter(m))
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RecordedAt == items[j].RecordedAt {
			return items[i].MeterId < items[j].MeterId
		}
		return items[i].RecordedAt < items[j].RecordedAt
	})

	start := 0
	if req.PageToken != "" {
		if p, err := strconv.Atoi(req.PageToken); err == nil && p >= 0 {
			start = p
		}
	}
	if start > len(items) {
		start = len(items)
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 100
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}

	return &rgsv1.ListMetersResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Meters: items[start:end], NextPageToken: next}, nil
}
