package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestRegistryGatewayParity_UpsertAndGet(t *testing.T) {
	registrySvc := NewRegistryService(ledgerFixedClock{now: time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterRegistryServiceHandlerServer(context.Background(), gwMux, registrySvc); err != nil {
		t.Fatalf("register registry gateway handlers: %v", err)
	}

	upsertReq := &rgsv1.UpsertEquipmentRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Equipment: &rgsv1.Equipment{
			EquipmentId: "eq-gw-1",
			Location:    "floor-1",
			Status:      rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE,
		},
		Reason: "commission",
	}
	body, err := protojson.Marshal(upsertReq)
	if err != nil {
		t.Fatalf("marshal upsert req: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/v1/registry/equipment/eq-gw-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("upsert http status: got=%d want=%d body=%s", rec.Result().StatusCode, http.StatusOK, rec.Body.String())
	}
	var httpUpsert rgsv1.UpsertEquipmentResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &httpUpsert); err != nil {
		t.Fatalf("unmarshal upsert response: %v", err)
	}

	directGet, err := registrySvc.GetEquipment(context.Background(), &rgsv1.GetEquipmentRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-gw-1",
	})
	if err != nil {
		t.Fatalf("direct get err: %v", err)
	}
	if httpUpsert.Equipment.GetEquipmentId() != directGet.Equipment.GetEquipmentId() {
		t.Fatalf("gateway/direct equipment id mismatch: http=%s direct=%s", httpUpsert.Equipment.GetEquipmentId(), directGet.Equipment.GetEquipmentId())
	}

	q := make(url.Values)
	q.Set("meta.actor.actorId", "op-1")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	getReq := httptest.NewRequest(http.MethodGet, "/v1/registry/equipment/eq-gw-1?"+q.Encode(), nil)
	getRec := httptest.NewRecorder()
	gwMux.ServeHTTP(getRec, getReq)
	if getRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("get http status: got=%d want=%d body=%s", getRec.Result().StatusCode, http.StatusOK, getRec.Body.String())
	}
	var httpGet rgsv1.GetEquipmentResponse
	if err := protojson.Unmarshal(getRec.Body.Bytes(), &httpGet); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if httpGet.Equipment.GetEquipmentId() != "eq-gw-1" {
		t.Fatalf("unexpected equipment id via gateway: %s", httpGet.Equipment.GetEquipmentId())
	}
}

func TestEventsGatewayParity_SubmitAndList(t *testing.T) {
	eventsSvc := NewEventsService(ledgerFixedClock{now: time.Date(2026, 2, 12, 12, 30, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterEventsServiceHandlerServer(context.Background(), gwMux, eventsSvc); err != nil {
		t.Fatalf("register events gateway handlers: %v", err)
	}

	submitReq := &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:              "ev-gw-1",
			EquipmentId:          "eq-1",
			EventCode:            "E900",
			LocalizedDescription: "gateway parity",
		},
	}
	body, err := protojson.Marshal(submitReq)
	if err != nil {
		t.Fatalf("marshal submit event req: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/events/significant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("submit event http status: got=%d want=%d body=%s", rec.Result().StatusCode, http.StatusOK, rec.Body.String())
	}
	var httpSubmit rgsv1.SubmitSignificantEventResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &httpSubmit); err != nil {
		t.Fatalf("unmarshal submit event response: %v", err)
	}

	directList, err := eventsSvc.ListEvents(context.Background(), &rgsv1.ListEventsRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("direct list events err: %v", err)
	}
	if len(directList.Events) != 1 {
		t.Fatalf("expected one event in direct list, got=%d", len(directList.Events))
	}
	if httpSubmit.Event.GetEventId() != directList.Events[0].GetEventId() {
		t.Fatalf("gateway/direct event id mismatch: http=%s direct=%s", httpSubmit.Event.GetEventId(), directList.Events[0].GetEventId())
	}

	mq := make(url.Values)
	mq.Set("meta.actor.actorId", "svc-1")
	mq.Set("meta.actor.actorType", "ACTOR_TYPE_SERVICE")
	meterReq := &rgsv1.SubmitMeterDeltaRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Meter: &rgsv1.MeterRecord{
			MeterId:      "m-gw-1",
			EquipmentId:  "eq-1",
			MeterLabel:   "coin_out",
			MonetaryUnit: "USD",
			DeltaMinor:   25,
		},
	}
	meterBody, err := protojson.Marshal(meterReq)
	if err != nil {
		t.Fatalf("marshal meter delta req: %v", err)
	}
	meterHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/events/meters/delta?"+mq.Encode(), bytes.NewReader(meterBody))
	meterHTTPReq.Header.Set("Content-Type", "application/json")
	meterRec := httptest.NewRecorder()
	gwMux.ServeHTTP(meterRec, meterHTTPReq)
	if meterRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("submit meter delta http status: got=%d want=%d body=%s", meterRec.Result().StatusCode, http.StatusOK, meterRec.Body.String())
	}

	lq := make(url.Values)
	lq.Set("meta.actor.actorId", "op-1")
	lq.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	listMetersReq := httptest.NewRequest(http.MethodGet, "/v1/events/meters?"+lq.Encode(), nil)
	listMetersRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listMetersRec, listMetersReq)
	if listMetersRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("list meters http status: got=%d want=%d body=%s", listMetersRec.Result().StatusCode, http.StatusOK, listMetersRec.Body.String())
	}
	var listMetersResp rgsv1.ListMetersResponse
	if err := protojson.Unmarshal(listMetersRec.Body.Bytes(), &listMetersResp); err != nil {
		t.Fatalf("unmarshal list meters response: %v", err)
	}
	if len(listMetersResp.Meters) != 1 || listMetersResp.Meters[0].GetMeterId() != "m-gw-1" {
		t.Fatalf("unexpected list meters result via gateway")
	}
}
