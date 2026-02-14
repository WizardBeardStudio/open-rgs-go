package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeardstudio/open-rgs-go/internal/platform/auth"
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

func TestConfigDisableInMemoryCacheSkipsDownloadMirror(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 17, 15, 0, 0, time.UTC)})
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	resp, err := svc.RecordDownloadLibraryChange(ctx, &rgsv1.RecordDownloadLibraryChangeRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Entry: &rgsv1.DownloadLibraryEntry{
			LibraryPath: "games/slot-b.pkg",
			Checksum:    "xyz789",
			Version:     "1.0.0",
			Action:      rgsv1.DownloadAction_DOWNLOAD_ACTION_ADD,
			Reason:      "initial load",
		},
	})
	if err != nil {
		t.Fatalf("record entry err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("record entry result not ok: %v", resp.Meta.GetResultCode())
	}

	list, err := svc.ListDownloadLibraryChanges(ctx, &rgsv1.ListDownloadLibraryChangesRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list download changes err: %v", err)
	}
	if len(list.Entries) != 0 {
		t.Fatalf("expected no in-memory download entries when cache disabled, got=%d", len(list.Entries))
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

func signDownloadEntryForTest(entry *rgsv1.DownloadLibraryEntry, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(downloadSignaturePayload(entry)))
	return base64.RawStdEncoding.EncodeToString(mac.Sum(nil))
}

func TestDownloadLibraryActivateRequiresValidSignature(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 17, 30, 0, 0, time.UTC)})
	svc.SetDownloadSignatureKeys(map[string][]byte{"k1": []byte("download-signing-secret")})
	ctx := context.Background()

	deniedMissingSig, err := svc.RecordDownloadLibraryChange(ctx, &rgsv1.RecordDownloadLibraryChangeRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Entry: &rgsv1.DownloadLibraryEntry{
			LibraryPath: "games/slot-a.pkg",
			Checksum:    "abc123",
			Version:     "2.0.0",
			Action:      rgsv1.DownloadAction_DOWNLOAD_ACTION_ACTIVATE,
			Reason:      "activate package",
		},
	})
	if err != nil {
		t.Fatalf("record activate missing signature err: %v", err)
	}
	if deniedMissingSig.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid for missing signature, got=%v", deniedMissingSig.Meta.GetResultCode())
	}

	validEntry := &rgsv1.DownloadLibraryEntry{
		LibraryPath: "games/slot-a.pkg",
		Checksum:    "abc123",
		Version:     "2.0.0",
		Action:      rgsv1.DownloadAction_DOWNLOAD_ACTION_ACTIVATE,
		Reason:      "activate package",
		SignerKid:   "k1",
	}
	validEntry.Signature = signDownloadEntryForTest(validEntry, []byte("download-signing-secret"))
	okResp, err := svc.RecordDownloadLibraryChange(ctx, &rgsv1.RecordDownloadLibraryChangeRequest{
		Meta:  meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Entry: validEntry,
	})
	if err != nil {
		t.Fatalf("record activate with signature err: %v", err)
	}
	if okResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok with valid signature, got=%v", okResp.Meta.GetResultCode())
	}

	badEntry := &rgsv1.DownloadLibraryEntry{
		LibraryPath: "games/slot-a.pkg",
		Checksum:    "abc123",
		Version:     "2.0.0",
		Action:      rgsv1.DownloadAction_DOWNLOAD_ACTION_ACTIVATE,
		Reason:      "activate package",
		SignerKid:   "k1",
		Signature:   "invalid-signature",
	}
	deniedBadSig, err := svc.RecordDownloadLibraryChange(ctx, &rgsv1.RecordDownloadLibraryChangeRequest{
		Meta:  meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Entry: badEntry,
	})
	if err != nil {
		t.Fatalf("record activate invalid signature err: %v", err)
	}
	if deniedBadSig.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied for invalid signature, got=%v", deniedBadSig.Meta.GetResultCode())
	}
}

func TestConfigActorMismatchDenied(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)})
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"})

	proposed, err := svc.ProposeConfigChange(ctx, &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespace: "security",
		ConfigKey:       "session_timeout",
		ProposedValue:   "600",
		Reason:          "mismatch",
	})
	if err != nil {
		t.Fatalf("propose err: %v", err)
	}
	if proposed.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied propose, got=%v", proposed.Meta.GetResultCode())
	}
	if proposed.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on propose, got=%q", proposed.Meta.GetDenialReason())
	}

	approved, err := svc.ApproveConfigChange(ctx, &rgsv1.ApproveConfigChangeRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: "chg-mismatch",
		Reason:   "mismatch",
	})
	if err != nil {
		t.Fatalf("approve err: %v", err)
	}
	if approved.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied approve, got=%v", approved.Meta.GetResultCode())
	}
	if approved.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on approve, got=%q", approved.Meta.GetDenialReason())
	}

	applied, err := svc.ApplyConfigChange(ctx, &rgsv1.ApplyConfigChangeRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: "chg-mismatch",
		Reason:   "mismatch",
	})
	if err != nil {
		t.Fatalf("apply err: %v", err)
	}
	if applied.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied apply, got=%v", applied.Meta.GetResultCode())
	}
	if applied.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on apply, got=%q", applied.Meta.GetDenialReason())
	}

	history, err := svc.ListConfigHistory(ctx, &rgsv1.ListConfigHistoryRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list history err: %v", err)
	}
	if history.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list history, got=%v", history.Meta.GetResultCode())
	}
	if history.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on list history, got=%q", history.Meta.GetDenialReason())
	}

	events := svc.AuditStore.Events()
	if len(events) == 0 {
		t.Fatalf("expected denied config audit events")
	}
	last := events[len(events)-1]
	if last.Action != "list_config_history" || last.Reason != "actor mismatch with token" {
		t.Fatalf("expected denied list_config_history audit with actor mismatch reason, got=%+v", last)
	}
}
