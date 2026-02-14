package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/clock"
	"google.golang.org/protobuf/proto"
)

type WageringService struct {
	rgsv1.UnimplementedWageringServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu                  sync.Mutex
	wagers              map[string]*rgsv1.Wager
	placeByIdempotency  map[string]*rgsv1.PlaceWagerResponse
	settleByIdempotency map[string]*rgsv1.SettleWagerResponse
	cancelByIdempotency map[string]*rgsv1.CancelWagerResponse
	nextWagerID         int64
	nextAuditID         int64
	db                  *sql.DB
	disableInMemCache   bool
}

func NewWageringService(clk clock.Clock, db ...*sql.DB) *WageringService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &WageringService{
		Clock:               clk,
		AuditStore:          audit.NewInMemoryStore(),
		wagers:              make(map[string]*rgsv1.Wager),
		placeByIdempotency:  make(map[string]*rgsv1.PlaceWagerResponse),
		settleByIdempotency: make(map[string]*rgsv1.SettleWagerResponse),
		cancelByIdempotency: make(map[string]*rgsv1.CancelWagerResponse),
		db:                  handle,
	}
}

func (s *WageringService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *WageringService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *WageringService) nextWagerIDLocked() string {
	s.nextWagerID++
	return "wager-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + "-" + strconv.FormatInt(s.nextWagerID, 10)
}

func (s *WageringService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "wagering-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *WageringService) SetDisableInMemoryIdempotencyCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemCache = disable
}

func (s *WageringService) useInMemoryCache() bool {
	if s == nil {
		return false
	}
	if s.dbEnabled() && s.disableInMemCache {
		return false
	}
	return true
}

func (s *WageringService) useInMemoryWagerMirror() bool {
	if s == nil {
		return false
	}
	if s.dbEnabled() && s.disableInMemCache {
		return false
	}
	return true
}

func (s *WageringService) appendAudit(meta *rgsv1.RequestMeta, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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
		ObjectType:   "wager",
		ObjectID:     objectID,
		Action:       action,
		Before:       before,
		After:        after,
		Result:       result,
		Reason:       reason,
		PartitionDay: now.Format("2006-01-02"),
	}
	if s.dbEnabled() {
		if err := appendAuditEventToDB(context.Background(), s.db, ev); err != nil {
			return err
		}
	}
	_, err := s.AuditStore.Append(ev)
	return err
}

func (s *WageringService) authorizePlace(ctx context.Context, meta *rgsv1.RequestMeta, playerID string) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	switch actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	case rgsv1.ActorType_ACTOR_TYPE_PLAYER:
		if actor.ActorId != playerID {
			return false, "player cannot place wager for another player"
		}
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func (s *WageringService) authorizeSettlement(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
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

func cloneWager(in *rgsv1.Wager) *rgsv1.Wager {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.Wager)
	return cp
}

func clonePlaceResponse(in *rgsv1.PlaceWagerResponse) *rgsv1.PlaceWagerResponse {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.PlaceWagerResponse)
	return cp
}

func cloneSettleResponse(in *rgsv1.SettleWagerResponse) *rgsv1.SettleWagerResponse {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.SettleWagerResponse)
	return cp
}

func cloneCancelResponse(in *rgsv1.CancelWagerResponse) *rgsv1.CancelWagerResponse {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.CancelWagerResponse)
	return cp
}

func (s *WageringService) PlaceWager(ctx context.Context, req *rgsv1.PlaceWagerRequest) (*rgsv1.PlaceWagerResponse, error) {
	if req == nil || req.PlayerId == "" || req.GameId == "" || invalidAmount(req.Stake) {
		return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "player_id, game_id, and valid stake are required")}, nil
	}
	if idempotency(req.Meta) == "" {
		return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}
	if ok, reason := s.authorizePlace(ctx, req.Meta, req.PlayerId); !ok {
		_ = s.appendAudit(req.Meta, "", "place_wager", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idem := idempotency(req.Meta)
	idemKey := req.PlayerId + "|place|" + idem
	requestHash := hashWageringRequest("place", req.PlayerId, req.GameId, req.Stake.GetCurrency(), strconv.FormatInt(req.Stake.GetAmountMinor(), 10))
	if s.useInMemoryCache() {
		if prev := s.placeByIdempotency[idemKey]; prev != nil {
			return clonePlaceResponse(prev), nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.PlaceWagerResponse
		found, err := s.loadIdempotencyResponse(ctx, "place", req.PlayerId, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryCache() {
				s.placeByIdempotency[idemKey] = clonePlaceResponse(&replay)
			}
			if replay.Wager != nil && s.useInMemoryWagerMirror() {
				s.wagers[replay.Wager.WagerId] = cloneWager(replay.Wager)
			}
			return &replay, nil
		}
	}

	now := s.now().Format(time.RFC3339Nano)
	wager := &rgsv1.Wager{
		WagerId:    s.nextWagerIDLocked(),
		PlayerId:   req.PlayerId,
		GameId:     req.GameId,
		Stake:      req.Stake,
		Status:     rgsv1.WagerStatus_WAGER_STATUS_PENDING,
		PlacedAt:   now,
		OutcomeRef: "",
	}
	if s.useInMemoryWagerMirror() {
		s.wagers[wager.WagerId] = wager
	}

	resp := &rgsv1.PlaceWagerResponse{
		Meta:  s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Wager: cloneWager(wager),
	}
	if s.useInMemoryCache() {
		s.placeByIdempotency[idemKey] = clonePlaceResponse(resp)
	}
	if err := s.persistWager(ctx, wager); err != nil {
		return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.persistIdempotencyResponse(ctx, "place", req.PlayerId, idem, requestHash, resp); err != nil {
		return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	after, _ := json.Marshal(wager)
	if err := s.appendAudit(req.Meta, wager.WagerId, "place_wager", []byte(`{}`), after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.PlaceWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return resp, nil
}

func (s *WageringService) SettleWager(ctx context.Context, req *rgsv1.SettleWagerRequest) (*rgsv1.SettleWagerResponse, error) {
	if req == nil || req.WagerId == "" || req.OutcomeRef == "" || invalidAmount(req.Payout) {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "wager_id, outcome_ref, and valid payout are required")}, nil
	}
	if idempotency(req.Meta) == "" {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}
	if ok, reason := s.authorizeSettlement(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, req.WagerId, "settle_wager", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idem := idempotency(req.Meta)
	idemKey := req.WagerId + "|settle|" + idem
	requestHash := hashWageringRequest("settle", req.WagerId, req.Payout.GetCurrency(), strconv.FormatInt(req.Payout.GetAmountMinor(), 10), req.OutcomeRef)
	if s.useInMemoryCache() {
		if prev := s.settleByIdempotency[idemKey]; prev != nil {
			return cloneSettleResponse(prev), nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.SettleWagerResponse
		found, err := s.loadIdempotencyResponse(ctx, "settle", req.WagerId, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryCache() {
				s.settleByIdempotency[idemKey] = cloneSettleResponse(&replay)
			}
			if replay.Wager != nil && s.useInMemoryWagerMirror() {
				s.wagers[replay.Wager.WagerId] = cloneWager(replay.Wager)
			}
			return &replay, nil
		}
	}

	var wager *rgsv1.Wager
	if s.useInMemoryWagerMirror() {
		wager = s.wagers[req.WagerId]
	}
	if wager == nil && s.dbEnabled() {
		var err error
		wager, err = s.getWager(ctx, req.WagerId)
		if err != nil {
			return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if wager != nil && s.useInMemoryWagerMirror() {
			s.wagers[wager.WagerId] = cloneWager(wager)
		}
	}
	if wager == nil {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "wager not found")}, nil
	}
	if wager.Status != rgsv1.WagerStatus_WAGER_STATUS_PENDING {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "wager is not pending")}, nil
	}
	before, _ := json.Marshal(wager)
	wager.Status = rgsv1.WagerStatus_WAGER_STATUS_SETTLED
	wager.Payout = req.Payout
	wager.OutcomeRef = req.OutcomeRef
	wager.SettledAt = s.now().Format(time.RFC3339Nano)
	after, _ := json.Marshal(wager)
	resp := &rgsv1.SettleWagerResponse{
		Meta:  s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Wager: cloneWager(wager),
	}
	if s.useInMemoryCache() {
		s.settleByIdempotency[idemKey] = cloneSettleResponse(resp)
	}
	if err := s.persistWager(ctx, wager); err != nil {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.persistIdempotencyResponse(ctx, "settle", req.WagerId, idem, requestHash, resp); err != nil {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.appendAudit(req.Meta, req.WagerId, "settle_wager", before, after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.SettleWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return resp, nil
}

func (s *WageringService) CancelWager(ctx context.Context, req *rgsv1.CancelWagerRequest) (*rgsv1.CancelWagerResponse, error) {
	if req == nil || req.WagerId == "" || req.Reason == "" {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.GetMeta(), rgsv1.ResultCode_RESULT_CODE_INVALID, "wager_id and reason are required")}, nil
	}
	if idempotency(req.Meta) == "" {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}
	if ok, reason := s.authorizeSettlement(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, req.WagerId, "cancel_wager", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idem := idempotency(req.Meta)
	idemKey := req.WagerId + "|cancel|" + idem
	requestHash := hashWageringRequest("cancel", req.WagerId, req.Reason)
	if s.useInMemoryCache() {
		if prev := s.cancelByIdempotency[idemKey]; prev != nil {
			return cloneCancelResponse(prev), nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.CancelWagerResponse
		found, err := s.loadIdempotencyResponse(ctx, "cancel", req.WagerId, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryCache() {
				s.cancelByIdempotency[idemKey] = cloneCancelResponse(&replay)
			}
			if replay.Wager != nil && s.useInMemoryWagerMirror() {
				s.wagers[replay.Wager.WagerId] = cloneWager(replay.Wager)
			}
			return &replay, nil
		}
	}

	var wager *rgsv1.Wager
	if s.useInMemoryWagerMirror() {
		wager = s.wagers[req.WagerId]
	}
	if wager == nil && s.dbEnabled() {
		var err error
		wager, err = s.getWager(ctx, req.WagerId)
		if err != nil {
			return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if wager != nil && s.useInMemoryWagerMirror() {
			s.wagers[wager.WagerId] = cloneWager(wager)
		}
	}
	if wager == nil {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "wager not found")}, nil
	}
	if wager.Status != rgsv1.WagerStatus_WAGER_STATUS_PENDING {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "wager is not pending")}, nil
	}
	before, _ := json.Marshal(wager)
	wager.Status = rgsv1.WagerStatus_WAGER_STATUS_CANCELED
	wager.CancelReason = req.Reason
	wager.CanceledAt = s.now().Format(time.RFC3339Nano)
	after, _ := json.Marshal(wager)
	resp := &rgsv1.CancelWagerResponse{
		Meta:  s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Wager: cloneWager(wager),
	}
	if s.useInMemoryCache() {
		s.cancelByIdempotency[idemKey] = cloneCancelResponse(resp)
	}
	if err := s.persistWager(ctx, wager); err != nil {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.persistIdempotencyResponse(ctx, "cancel", req.WagerId, idem, requestHash, resp); err != nil {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.appendAudit(req.Meta, req.WagerId, "cancel_wager", before, after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.CancelWagerResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return resp, nil
}
