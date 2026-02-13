package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"google.golang.org/protobuf/proto"
)

type PromotionsService struct {
	rgsv1.UnimplementedPromotionsServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu                   sync.Mutex
	bonusTx              map[string]*rgsv1.BonusTransaction
	bonusOrder           []string
	awards               map[string]*rgsv1.PromotionalAward
	awardOrder           []string
	nextBonusID          int64
	nextAwardID          int64
	nextAuditID          int64
	db                   *sql.DB
	disableInMemoryCache bool
}

func NewPromotionsService(clk clock.Clock, db ...*sql.DB) *PromotionsService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &PromotionsService{
		Clock:      clk,
		AuditStore: audit.NewInMemoryStore(),
		bonusTx:    make(map[string]*rgsv1.BonusTransaction),
		awards:     make(map[string]*rgsv1.PromotionalAward),
		db:         handle,
	}
}

func (s *PromotionsService) SetDisableInMemoryCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemoryCache = disable
}

func (s *PromotionsService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *PromotionsService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *PromotionsService) authorize(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
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

func (s *PromotionsService) nextBonusIDLocked() string {
	s.nextBonusID++
	return "bonus-" + strconv.FormatInt(s.nextBonusID, 10)
}

func (s *PromotionsService) nextAwardIDLocked() string {
	s.nextAwardID++
	return "award-" + strconv.FormatInt(s.nextAwardID, 10)
}

func (s *PromotionsService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "promotions-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *PromotionsService) appendAudit(meta *rgsv1.RequestMeta, objectType, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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
	ev := audit.Event{
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
	}
	if s.db != nil {
		if err := appendAuditEventToDB(context.Background(), s.db, ev); err != nil {
			return err
		}
	}
	_, err := s.AuditStore.Append(ev)
	return err
}

func cloneBonusTx(in *rgsv1.BonusTransaction) *rgsv1.BonusTransaction {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.BonusTransaction)
	return cp
}

func cloneAward(in *rgsv1.PromotionalAward) *rgsv1.PromotionalAward {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.PromotionalAward)
	return cp
}

func (s *PromotionsService) RecordBonusTransaction(ctx context.Context, req *rgsv1.RecordBonusTransactionRequest) (*rgsv1.RecordBonusTransactionResponse, error) {
	if req == nil || req.Transaction == nil || req.Transaction.EquipmentId == "" || req.Transaction.PlayerId == "" || invalidAmount(req.Transaction.Amount) {
		return &rgsv1.RecordBonusTransactionResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "transaction requires equipment_id, player_id, and positive amount")}, nil
	}
	if _, ok := parseRFC3339Strict(req.Transaction.OccurredAt); req.Transaction.OccurredAt != "" && !ok {
		return &rgsv1.RecordBonusTransactionResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid occurred_at")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "bonus_transaction", "", "record_bonus_transaction", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.RecordBonusTransactionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := cloneBonusTx(req.Transaction)
	if tx.BonusTransactionId == "" {
		tx.BonusTransactionId = s.nextBonusIDLocked()
	}
	if tx.OccurredAt == "" {
		tx.OccurredAt = s.now().Format(time.RFC3339Nano)
	}
	if !s.disableInMemoryCache {
		s.bonusTx[tx.BonusTransactionId] = cloneBonusTx(tx)
		s.bonusOrder = append(s.bonusOrder, tx.BonusTransactionId)
	}
	if err := s.persistBonusTransaction(ctx, tx); err != nil {
		return &rgsv1.RecordBonusTransactionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	after, _ := json.Marshal(tx)
	if err := s.appendAudit(req.Meta, "bonus_transaction", tx.BonusTransactionId, "record_bonus_transaction", []byte(`{}`), after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.RecordBonusTransactionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}

	return &rgsv1.RecordBonusTransactionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Transaction: tx}, nil
}

func (s *PromotionsService) ListRecentBonusTransactions(ctx context.Context, req *rgsv1.ListRecentBonusTransactionsRequest) (*rgsv1.ListRecentBonusTransactionsResponse, error) {
	if req == nil {
		req = &rgsv1.ListRecentBonusTransactionsRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if req.Limit < 0 {
		return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid limit")}, nil
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid limit")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		rows, err := s.listBonusTransactionsFromDB(ctx, req.EquipmentId, limit)
		if err != nil {
			return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Transactions: rows}, nil
	}
	if s.disableInMemoryCache {
		return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Transactions: nil}, nil
	}

	out := make([]*rgsv1.BonusTransaction, 0, limit)
	for i := len(s.bonusOrder) - 1; i >= 0 && len(out) < limit; i-- {
		tx := s.bonusTx[s.bonusOrder[i]]
		if tx == nil {
			continue
		}
		if req.EquipmentId != "" && tx.EquipmentId != req.EquipmentId {
			continue
		}
		out = append(out, cloneBonusTx(tx))
	}
	return &rgsv1.ListRecentBonusTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Transactions: out}, nil
}

func (s *PromotionsService) RecordPromotionalAward(ctx context.Context, req *rgsv1.RecordPromotionalAwardRequest) (*rgsv1.RecordPromotionalAwardResponse, error) {
	if req == nil || req.Award == nil || req.Award.PlayerId == "" || !validPromotionalAwardType(req.Award.AwardType) || invalidAmount(req.Award.Amount) {
		return &rgsv1.RecordPromotionalAwardResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "award requires player_id, award_type, and positive amount")}, nil
	}
	if _, ok := parseRFC3339Strict(req.Award.OccurredAt); req.Award.OccurredAt != "" && !ok {
		return &rgsv1.RecordPromotionalAwardResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid occurred_at")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "promotional_award", "", "record_promotional_award", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.RecordPromotionalAwardResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	award := cloneAward(req.Award)
	if award.PromotionalAwardId == "" {
		award.PromotionalAwardId = s.nextAwardIDLocked()
	}
	if award.OccurredAt == "" {
		award.OccurredAt = s.now().Format(time.RFC3339Nano)
	}
	if !s.disableInMemoryCache {
		s.awards[award.PromotionalAwardId] = cloneAward(award)
		s.awardOrder = append(s.awardOrder, award.PromotionalAwardId)
	}
	if err := s.persistPromotionalAward(ctx, award); err != nil {
		return &rgsv1.RecordPromotionalAwardResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	after, _ := json.Marshal(award)
	if err := s.appendAudit(req.Meta, "promotional_award", award.PromotionalAwardId, "record_promotional_award", []byte(`{}`), after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.RecordPromotionalAwardResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.RecordPromotionalAwardResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Award: award}, nil
}

func (s *PromotionsService) ListPromotionalAwards(ctx context.Context, req *rgsv1.ListPromotionalAwardsRequest) (*rgsv1.ListPromotionalAwardsResponse, error) {
	if req == nil {
		req = &rgsv1.ListPromotionalAwardsRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if req.PageSize < 0 {
		return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_size")}, nil
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 25
	}
	if size > 100 {
		return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_size")}, nil
	}
	start := 0
	if req.PageToken != "" {
		n, err := strconv.Atoi(req.PageToken)
		if err != nil || n < 0 {
			return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_token")}, nil
		}
		start = n
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		rows, next, err := s.listPromotionalAwardsFromDB(ctx, req.PlayerId, req.CampaignId, size, start)
		if err != nil {
			return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Awards: rows, NextPageToken: next}, nil
	}
	if s.disableInMemoryCache {
		return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Awards: nil}, nil
	}

	items := make([]*rgsv1.PromotionalAward, 0, len(s.awardOrder))
	for i := len(s.awardOrder) - 1; i >= 0; i-- {
		aw := s.awards[s.awardOrder[i]]
		if aw == nil {
			continue
		}
		if req.PlayerId != "" && aw.PlayerId != req.PlayerId {
			continue
		}
		if req.CampaignId != "" && aw.CampaignId != req.CampaignId {
			continue
		}
		items = append(items, cloneAward(aw))
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return &rgsv1.ListPromotionalAwardsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Awards: items[start:end], NextPageToken: next}, nil
}

type UISystemOverlayService struct {
	rgsv1.UnimplementedUISystemOverlayServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu                   sync.Mutex
	events               map[string]*rgsv1.SystemWindowEvent
	eventOrder           []string
	nextEventID          int64
	nextAuditID          int64
	db                   *sql.DB
	disableInMemoryCache bool
}

func NewUISystemOverlayService(clk clock.Clock, db ...*sql.DB) *UISystemOverlayService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &UISystemOverlayService{
		Clock:      clk,
		AuditStore: audit.NewInMemoryStore(),
		events:     make(map[string]*rgsv1.SystemWindowEvent),
		db:         handle,
	}
}

func (s *UISystemOverlayService) SetDisableInMemoryCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemoryCache = disable
}

func (s *UISystemOverlayService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *UISystemOverlayService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *UISystemOverlayService) authorize(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
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

func (s *UISystemOverlayService) nextEventIDLocked() string {
	s.nextEventID++
	return "win-evt-" + strconv.FormatInt(s.nextEventID, 10)
}

func (s *UISystemOverlayService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "ui-overlay-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *UISystemOverlayService) appendAudit(meta *rgsv1.RequestMeta, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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
	ev := audit.Event{
		AuditID:      s.nextAuditIDLocked(),
		OccurredAt:   now,
		RecordedAt:   now,
		ActorID:      actorID,
		ActorType:    actorType,
		ObjectType:   "system_window_event",
		ObjectID:     objectID,
		Action:       action,
		Before:       before,
		After:        after,
		Result:       result,
		Reason:       reason,
		PartitionDay: now.Format("2006-01-02"),
	}
	if s.db != nil {
		if err := appendAuditEventToDB(context.Background(), s.db, ev); err != nil {
			return err
		}
	}
	_, err := s.AuditStore.Append(ev)
	return err
}

func cloneSystemWindowEvent(in *rgsv1.SystemWindowEvent) *rgsv1.SystemWindowEvent {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.SystemWindowEvent)
	return cp
}

func parseRFC3339OrZero(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return ts.UTC()
}

func parseRFC3339Strict(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, true
	}
	ts, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}

func validPromotionalAwardType(t rgsv1.PromotionalAwardType) bool {
	switch t {
	case rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
		rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_MATCH_BONUS,
		rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_LOYALTY_POINTS,
		rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_NON_CASHABLE_CREDIT:
		return true
	default:
		return false
	}
}

func validSystemWindowEventType(t rgsv1.SystemWindowEventType) bool {
	switch t {
	case rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
		rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_CLOSED,
		rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_DECLINED,
		rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_TIMED_OUT:
		return true
	default:
		return false
	}
}

func (s *UISystemOverlayService) SubmitSystemWindowEvent(ctx context.Context, req *rgsv1.SubmitSystemWindowEventRequest) (*rgsv1.SubmitSystemWindowEventResponse, error) {
	if req == nil || req.Event == nil || req.Event.EquipmentId == "" || req.Event.WindowId == "" || !validSystemWindowEventType(req.Event.EventType) {
		return &rgsv1.SubmitSystemWindowEventResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "event requires equipment_id, window_id, and event_type")}, nil
	}
	if _, ok := parseRFC3339Strict(req.Event.EventTime); req.Event.EventTime != "" && !ok {
		return &rgsv1.SubmitSystemWindowEventResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid event_time")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "", "submit_system_window_event", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.SubmitSystemWindowEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ev := cloneSystemWindowEvent(req.Event)
	if ev.EventId == "" {
		ev.EventId = s.nextEventIDLocked()
	}
	if ev.EventTime == "" {
		ev.EventTime = s.now().Format(time.RFC3339Nano)
	}
	if !s.disableInMemoryCache {
		s.events[ev.EventId] = cloneSystemWindowEvent(ev)
		s.eventOrder = append(s.eventOrder, ev.EventId)
	}
	if err := s.persistSystemWindowEvent(ctx, ev); err != nil {
		return &rgsv1.SubmitSystemWindowEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	after, _ := json.Marshal(ev)
	if err := s.appendAudit(req.Meta, ev.EventId, "submit_system_window_event", []byte(`{}`), after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.SubmitSystemWindowEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}

	return &rgsv1.SubmitSystemWindowEventResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Event: ev}, nil
}

func (s *UISystemOverlayService) ListSystemWindowEvents(ctx context.Context, req *rgsv1.ListSystemWindowEventsRequest) (*rgsv1.ListSystemWindowEventsResponse, error) {
	if req == nil {
		req = &rgsv1.ListSystemWindowEventsRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if req.PageSize < 0 {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_size")}, nil
	}
	start := 0
	if req.PageToken != "" {
		v, err := strconv.Atoi(req.PageToken)
		if err != nil || v < 0 {
			return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_token")}, nil
		}
		start = v
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 50
	}
	if size > 200 {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid page_size")}, nil
	}
	fromTS, ok := parseRFC3339Strict(req.FromTime)
	if !ok {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid from_time")}, nil
	}
	toTS, ok := parseRFC3339Strict(req.ToTime)
	if !ok {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "invalid to_time")}, nil
	}
	if !fromTS.IsZero() && !toTS.IsZero() && fromTS.After(toTS) {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "from_time must be <= to_time")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		rows, next, err := s.listSystemWindowEventsFromDB(ctx, req.EquipmentId, fromTS, toTS, size, start)
		if err != nil {
			return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: rows, NextPageToken: next}, nil
	}
	if s.disableInMemoryCache {
		return &rgsv1.ListSystemWindowEventsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Events: nil, NextPageToken: ""}, nil
	}

	filtered := make([]*rgsv1.SystemWindowEvent, 0, len(s.eventOrder))
	for i := len(s.eventOrder) - 1; i >= 0; i-- {
		ev := s.events[s.eventOrder[i]]
		if ev == nil {
			continue
		}
		if req.EquipmentId != "" && ev.EquipmentId != req.EquipmentId {
			continue
		}
		evTS := parseRFC3339OrZero(ev.EventTime)
		if !fromTS.IsZero() && evTS.Before(fromTS) {
			continue
		}
		if !toTS.IsZero() && evTS.After(toTS) {
			continue
		}
		filtered = append(filtered, cloneSystemWindowEvent(ev))
	}
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	next := ""
	if end < len(filtered) {
		next = strconv.Itoa(end)
	}
	return &rgsv1.ListSystemWindowEventsResponse{
		Meta:          s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Events:        filtered[start:end],
		NextPageToken: next,
	}, nil
}
