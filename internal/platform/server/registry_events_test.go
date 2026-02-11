package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
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
