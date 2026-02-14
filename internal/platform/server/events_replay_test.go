package server

import (
	"context"
	"reflect"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
)

func TestEventsReplayDeterministicFinalState_OutOfOrderSignificantEvents(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 13, 0, 0, 0, time.UTC)}
	svcA := NewEventsService(clk)
	svcB := NewEventsService(clk)
	ctx := context.Background()

	payloads := []*rgsv1.SignificantEvent{
		{EventId: "ev-2", EquipmentId: "eq-1", EventCode: "E2", LocalizedDescription: "second", OccurredAt: "2026-02-12T12:00:02Z"},
		{EventId: "ev-1", EquipmentId: "eq-1", EventCode: "E1", LocalizedDescription: "first", OccurredAt: "2026-02-12T12:00:01Z"},
		{EventId: "ev-3", EquipmentId: "eq-1", EventCode: "E3", LocalizedDescription: "third", OccurredAt: "2026-02-12T12:00:03Z"},
	}

	for _, e := range payloads {
		_, err := svcA.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""), Event: e})
		if err != nil {
			t.Fatalf("svcA submit event %s: %v", e.EventId, err)
		}
	}

	for i := len(payloads) - 1; i >= 0; i-- {
		_, err := svcB.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""), Event: payloads[i]})
		if err != nil {
			t.Fatalf("svcB submit event %s: %v", payloads[i].EventId, err)
		}
	}

	listA, err := svcA.ListEvents(ctx, &rgsv1.ListEventsRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("svcA list events: %v", err)
	}
	listB, err := svcB.ListEvents(ctx, &rgsv1.ListEventsRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("svcB list events: %v", err)
	}

	idsA := make([]string, 0, len(listA.Events))
	for _, e := range listA.Events {
		idsA = append(idsA, e.EventId)
	}
	idsB := make([]string, 0, len(listB.Events))
	for _, e := range listB.Events {
		idsB = append(idsB, e.EventId)
	}

	if !reflect.DeepEqual(idsA, idsB) {
		t.Fatalf("deterministic replay failed for events: A=%v B=%v", idsA, idsB)
	}
}

func TestEventsReplayDeterministicFinalState_OutOfOrderMeters(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 13, 5, 0, 0, time.UTC)}
	svcA := NewEventsService(clk)
	svcB := NewEventsService(clk)
	ctx := context.Background()

	payloads := []*rgsv1.MeterRecord{
		{MeterId: "m-2", EquipmentId: "eq-1", MeterLabel: "coin_in", MonetaryUnit: "USD", ValueMinor: 110, OccurredAt: "2026-02-12T12:01:02Z"},
		{MeterId: "m-1", EquipmentId: "eq-1", MeterLabel: "coin_in", MonetaryUnit: "USD", ValueMinor: 100, OccurredAt: "2026-02-12T12:01:01Z"},
		{MeterId: "m-3", EquipmentId: "eq-1", MeterLabel: "coin_in", MonetaryUnit: "USD", ValueMinor: 120, OccurredAt: "2026-02-12T12:01:03Z"},
	}

	for _, m := range payloads {
		_, err := svcA.SubmitMeterSnapshot(ctx, &rgsv1.SubmitMeterSnapshotRequest{Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""), Meter: m})
		if err != nil {
			t.Fatalf("svcA submit meter %s: %v", m.MeterId, err)
		}
	}

	for i := len(payloads) - 1; i >= 0; i-- {
		_, err := svcB.SubmitMeterSnapshot(ctx, &rgsv1.SubmitMeterSnapshotRequest{Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""), Meter: payloads[i]})
		if err != nil {
			t.Fatalf("svcB submit meter %s: %v", payloads[i].MeterId, err)
		}
	}

	listA, err := svcA.ListMeters(ctx, &rgsv1.ListMetersRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("svcA list meters: %v", err)
	}
	listB, err := svcB.ListMeters(ctx, &rgsv1.ListMetersRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("svcB list meters: %v", err)
	}

	idsA := make([]string, 0, len(listA.Meters))
	for _, m := range listA.Meters {
		idsA = append(idsA, m.MeterId)
	}
	idsB := make([]string, 0, len(listB.Meters))
	for _, m := range listB.Meters {
		idsB = append(idsB, m.MeterId)
	}

	if !reflect.DeepEqual(idsA, idsB) {
		t.Fatalf("deterministic replay failed for meters: A=%v B=%v", idsA, idsB)
	}
}
