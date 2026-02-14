package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeardstudio/open-rgs-go/internal/platform/auth"
)

func TestRegistryUpsertAndListDeterministic(t *testing.T) {
	svc := NewRegistryService(ledgerFixedClock{now: time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)})
	ctx := context.Background()

	_, err := svc.UpsertEquipment(ctx, &rgsv1.UpsertEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-b",
			Location:    "room-b",
			Status:      rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE,
		},
		Reason: "register",
	})
	if err != nil {
		t.Fatalf("upsert eq-b: %v", err)
	}
	_, err = svc.UpsertEquipment(ctx, &rgsv1.UpsertEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-a",
			Location:    "room-a",
			Status:      rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE,
		},
		Reason: "register",
	})
	if err != nil {
		t.Fatalf("upsert eq-a: %v", err)
	}

	listed, err := svc.ListEquipment(ctx, &rgsv1.ListEquipmentRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("list equipment: %v", err)
	}
	if len(listed.Equipment) != 2 {
		t.Fatalf("expected 2 equipment entries, got=%d", len(listed.Equipment))
	}
	if listed.Equipment[0].EquipmentId != "eq-a" || listed.Equipment[1].EquipmentId != "eq-b" {
		t.Fatalf("equipment list not deterministic by id: got=%s,%s", listed.Equipment[0].EquipmentId, listed.Equipment[1].EquipmentId)
	}
}

func TestRegistryAuthorizationDeniedForPlayer(t *testing.T) {
	svc := NewRegistryService(ledgerFixedClock{now: time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)})
	ctx := context.Background()

	resp, err := svc.UpsertEquipment(ctx, &rgsv1.UpsertEquipmentRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-x",
		},
	})
	if err != nil {
		t.Fatalf("upsert equipment err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player upsert, got=%v", resp.Meta.GetResultCode())
	}
}

func TestRegistryDisableInMemoryCacheSkipsMirror(t *testing.T) {
	svc := NewRegistryService(ledgerFixedClock{now: time.Date(2026, 2, 12, 10, 5, 0, 0, time.UTC)})
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	upsert, err := svc.UpsertEquipment(ctx, &rgsv1.UpsertEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-disabled",
			Location:    "room-x",
			Status:      rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE,
		},
		Reason: "register",
	})
	if err != nil {
		t.Fatalf("upsert equipment: %v", err)
	}
	if upsert.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected upsert ok, got=%v", upsert.Meta.GetResultCode())
	}

	got, err := svc.GetEquipment(ctx, &rgsv1.GetEquipmentRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-disabled",
	})
	if err != nil {
		t.Fatalf("get equipment: %v", err)
	}
	if got.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected not found in memory-disabled mode, got=%v", got.Meta.GetResultCode())
	}

	list, err := svc.ListEquipment(ctx, &rgsv1.ListEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list equipment: %v", err)
	}
	if len(list.Equipment) != 0 {
		t.Fatalf("expected no in-memory equipment entries when cache disabled, got=%d", len(list.Equipment))
	}
}

func TestEventsSubmitAndList(t *testing.T) {
	svc := NewEventsService(ledgerFixedClock{now: time.Date(2026, 2, 12, 10, 30, 0, 0, time.UTC)})
	ctx := context.Background()

	evResp, err := svc.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:              "ev-1",
			EquipmentId:          "eq-1",
			EventCode:            "E100",
			LocalizedDescription: "door open",
		},
	})
	if err != nil {
		t.Fatalf("submit event: %v", err)
	}
	if evResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected event submit ok, got=%v", evResp.Meta.GetResultCode())
	}

	meterResp, err := svc.SubmitMeterSnapshot(ctx, &rgsv1.SubmitMeterSnapshotRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Meter: &rgsv1.MeterRecord{
			MeterId:      "m-1",
			EquipmentId:  "eq-1",
			MeterLabel:   "coin_in",
			MonetaryUnit: "USD",
			ValueMinor:   1234,
		},
	})
	if err != nil {
		t.Fatalf("submit meter: %v", err)
	}
	if meterResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected meter submit ok, got=%v", meterResp.Meta.GetResultCode())
	}

	events, err := svc.ListEvents(ctx, &rgsv1.ListEventsRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Events) != 1 || events.Events[0].EventId != "ev-1" {
		t.Fatalf("unexpected events list contents")
	}

	meters, err := svc.ListMeters(ctx, &rgsv1.ListMetersRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("list meters: %v", err)
	}
	if len(meters.Meters) != 1 || meters.Meters[0].MeterId != "m-1" {
		t.Fatalf("unexpected meters list contents")
	}
}

func TestEventsBufferExhaustionDisablesIngress(t *testing.T) {
	svc := NewEventsService(ledgerFixedClock{now: time.Date(2026, 2, 12, 11, 0, 0, 0, time.UTC)})
	svc.bufferCap = 0

	resp, err := svc.SubmitSignificantEvent(context.Background(), &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:     "ev-overflow",
			EquipmentId: "eq-1",
		},
	})
	if err != nil {
		t.Fatalf("submit with exhausted buffer err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied when buffer exhausted, got=%v", resp.Meta.GetResultCode())
	}
	if !svc.disabled {
		t.Fatalf("expected service ingestion to be disabled after exhaustion")
	}
}

func TestEventsDisableInMemoryCacheSkipsMirrors(t *testing.T) {
	svc := NewEventsService(ledgerFixedClock{now: time.Date(2026, 2, 12, 11, 15, 0, 0, time.UTC)})
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	_, err := svc.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:     "ev-disabled",
			EquipmentId: "eq-1",
			EventCode:   "E101",
		},
	})
	if err != nil {
		t.Fatalf("submit event err: %v", err)
	}

	_, err = svc.SubmitMeterSnapshot(ctx, &rgsv1.SubmitMeterSnapshotRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Meter: &rgsv1.MeterRecord{
			MeterId:      "m-disabled",
			EquipmentId:  "eq-1",
			MeterLabel:   "coin_in",
			MonetaryUnit: "USD",
			ValueMinor:   1,
		},
	})
	if err != nil {
		t.Fatalf("submit meter err: %v", err)
	}

	events, err := svc.ListEvents(ctx, &rgsv1.ListEventsRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("list events err: %v", err)
	}
	if len(events.Events) != 0 {
		t.Fatalf("expected no in-memory events when cache disabled, got=%d", len(events.Events))
	}

	meters, err := svc.ListMeters(ctx, &rgsv1.ListMetersRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("list meters err: %v", err)
	}
	if len(meters.Meters) != 0 {
		t.Fatalf("expected no in-memory meters when cache disabled, got=%d", len(meters.Meters))
	}
}

func TestRegistryActorMismatchDenied(t *testing.T) {
	svc := NewRegistryService(ledgerFixedClock{now: time.Date(2026, 2, 12, 11, 30, 0, 0, time.UTC)})
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"})

	upsertResp, err := svc.UpsertEquipment(ctx, &rgsv1.UpsertEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-mismatch",
			Location:    "room-x",
			Status:      rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE,
		},
	})
	if err != nil {
		t.Fatalf("upsert equipment err: %v", err)
	}
	if upsertResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied upsert mismatch, got=%v", upsertResp.GetMeta().GetResultCode())
	}
	if upsertResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on upsert, got=%q", upsertResp.GetMeta().GetDenialReason())
	}

	_, err = svc.UpsertEquipment(context.Background(), &rgsv1.UpsertEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-seed",
			Location:    "room-seed",
			Status:      rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE,
		},
	})
	if err != nil {
		t.Fatalf("seed upsert equipment err: %v", err)
	}

	getResp, err := svc.GetEquipment(ctx, &rgsv1.GetEquipmentRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-seed",
	})
	if err != nil {
		t.Fatalf("get equipment err: %v", err)
	}
	if getResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied get mismatch, got=%v", getResp.GetMeta().GetResultCode())
	}
	if getResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on get, got=%q", getResp.GetMeta().GetDenialReason())
	}

	listResp, err := svc.ListEquipment(ctx, &rgsv1.ListEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list equipment err: %v", err)
	}
	if listResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list mismatch, got=%v", listResp.GetMeta().GetResultCode())
	}
	if listResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on list, got=%q", listResp.GetMeta().GetDenialReason())
	}
}

func TestEventsActorMismatchDenied(t *testing.T) {
	svc := NewEventsService(ledgerFixedClock{now: time.Date(2026, 2, 12, 11, 45, 0, 0, time.UTC)})
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-svc", Type: "ACTOR_TYPE_SERVICE"})

	submitResp, err := svc.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:     "ev-mismatch",
			EquipmentId: "eq-1",
			EventCode:   "E200",
		},
	})
	if err != nil {
		t.Fatalf("submit event err: %v", err)
	}
	if submitResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied submit mismatch, got=%v", submitResp.GetMeta().GetResultCode())
	}
	if submitResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on submit, got=%q", submitResp.GetMeta().GetDenialReason())
	}

	listResp, err := svc.ListEvents(ctx, &rgsv1.ListEventsRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
	})
	if err != nil {
		t.Fatalf("list events err: %v", err)
	}
	if listResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list mismatch, got=%v", listResp.GetMeta().GetResultCode())
	}
	if listResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on list events, got=%q", listResp.GetMeta().GetDenialReason())
	}

	snapshotResp, err := svc.SubmitMeterSnapshot(ctx, &rgsv1.SubmitMeterSnapshotRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Meter: &rgsv1.MeterRecord{
			MeterId:      "m-mismatch-snapshot",
			EquipmentId:  "eq-1",
			MeterLabel:   "coin_in",
			MonetaryUnit: "USD",
			ValueMinor:   1,
		},
	})
	if err != nil {
		t.Fatalf("submit meter snapshot err: %v", err)
	}
	if snapshotResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied snapshot mismatch, got=%v", snapshotResp.GetMeta().GetResultCode())
	}
	if snapshotResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on submit meter snapshot, got=%q", snapshotResp.GetMeta().GetDenialReason())
	}

	deltaResp, err := svc.SubmitMeterDelta(ctx, &rgsv1.SubmitMeterDeltaRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Meter: &rgsv1.MeterRecord{
			MeterId:      "m-mismatch-delta",
			EquipmentId:  "eq-1",
			MeterLabel:   "coin_out",
			MonetaryUnit: "USD",
			DeltaMinor:   1,
		},
	})
	if err != nil {
		t.Fatalf("submit meter delta err: %v", err)
	}
	if deltaResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied delta mismatch, got=%v", deltaResp.GetMeta().GetResultCode())
	}
	if deltaResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on submit meter delta, got=%q", deltaResp.GetMeta().GetDenialReason())
	}

	metersResp, err := svc.ListMeters(ctx, &rgsv1.ListMetersRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
	})
	if err != nil {
		t.Fatalf("list meters err: %v", err)
	}
	if metersResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list meters mismatch, got=%v", metersResp.GetMeta().GetResultCode())
	}
	if metersResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on list meters, got=%q", metersResp.GetMeta().GetDenialReason())
	}
}
