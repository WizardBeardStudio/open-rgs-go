package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func TestConfigChangeWorkflow(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 17, 0, 0, 0, time.UTC)})
	ctx := context.Background()

	proposed, err := svc.ProposeConfigChange(ctx, &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespace: "security",
		ConfigKey:       "session_timeout",
		ProposedValue:   "900",
		Reason:          "tighten timeout",
	})
	if err != nil {
		t.Fatalf("propose err: %v", err)
	}
	if proposed.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("propose result not ok: %v", proposed.Meta.GetResultCode())
	}

	approved, err := svc.ApproveConfigChange(ctx, &rgsv1.ApproveConfigChangeRequest{
		Meta:     meta("op-2", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: proposed.Change.ChangeId,
		Reason:   "approved by second operator",
	})
	if err != nil {
		t.Fatalf("approve err: %v", err)
	}
	if approved.Change.GetStatus() != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPROVED {
		t.Fatalf("expected approved status, got=%v", approved.Change.GetStatus())
	}

	applied, err := svc.ApplyConfigChange(ctx, &rgsv1.ApplyConfigChangeRequest{
		Meta:     meta("op-3", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: proposed.Change.ChangeId,
		Reason:   "maintenance window",
	})
	if err != nil {
		t.Fatalf("apply err: %v", err)
	}
	if applied.Change.GetStatus() != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPLIED {
		t.Fatalf("expected applied status, got=%v", applied.Change.GetStatus())
	}

	history, err := svc.ListConfigHistory(ctx, &rgsv1.ListConfigHistoryRequest{
		Meta:                  meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespaceFilter: "security",
	})
	if err != nil {
		t.Fatalf("list history err: %v", err)
	}
	if len(history.Changes) != 1 || history.Changes[0].ChangeId != proposed.Change.ChangeId {
		t.Fatalf("unexpected config history results")
	}
}

func TestDownloadLibraryChangeRecording(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 17, 10, 0, 0, time.UTC)})
	ctx := context.Background()

	first, err := svc.RecordDownloadLibraryChange(ctx, &rgsv1.RecordDownloadLibraryChangeRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Entry: &rgsv1.DownloadLibraryEntry{
			LibraryPath: "games/slot-a.pkg",
			Checksum:    "abc123",
			Version:     "1.0.0",
			Action:      rgsv1.DownloadAction_DOWNLOAD_ACTION_ADD,
			Reason:      "initial load",
		},
	})
	if err != nil {
		t.Fatalf("record first entry err: %v", err)
	}
	if first.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("record first entry result not ok: %v", first.Meta.GetResultCode())
	}

	_, _ = svc.RecordDownloadLibraryChange(ctx, &rgsv1.RecordDownloadLibraryChangeRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Entry: &rgsv1.DownloadLibraryEntry{
			LibraryPath: "games/slot-a.pkg",
			Checksum:    "def456",
			Version:     "1.1.0",
			Action:      rgsv1.DownloadAction_DOWNLOAD_ACTION_UPDATE,
			Reason:      "patch",
		},
	})

	list, err := svc.ListDownloadLibraryChanges(ctx, &rgsv1.ListDownloadLibraryChangesRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list download changes err: %v", err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("expected 2 download entries, got=%d", len(list.Entries))
	}
}

func TestConfigDeniedForPlayer(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 17, 20, 0, 0, time.UTC)})
	resp, err := svc.ProposeConfigChange(context.Background(), &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		ConfigNamespace: "security",
		ConfigKey:       "session_timeout",
		ProposedValue:   "900",
	})
	if err != nil {
		t.Fatalf("propose err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied for player, got=%v", resp.Meta.GetResultCode())
	}
}
