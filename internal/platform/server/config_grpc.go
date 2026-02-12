package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"google.golang.org/protobuf/proto"
)

type ConfigService struct {
	rgsv1.UnimplementedConfigServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu sync.Mutex

	changes      map[string]*rgsv1.ConfigChange
	changeOrder  []string
	nextChangeID int64

	currentValues map[string]string

	downloadEntries      map[string]*rgsv1.DownloadLibraryEntry
	downloadOrder        []string
	nextEntryID          int64
	nextAuditID          int64
	db                   *sql.DB
	downloadSigKeys      map[string][]byte
	disableInMemoryCache bool
}

func NewConfigService(clk clock.Clock, db ...*sql.DB) *ConfigService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &ConfigService{
		Clock:           clk,
		AuditStore:      audit.NewInMemoryStore(),
		changes:         make(map[string]*rgsv1.ConfigChange),
		currentValues:   make(map[string]string),
		downloadEntries: make(map[string]*rgsv1.DownloadLibraryEntry),
		downloadSigKeys: make(map[string][]byte),
		db:              handle,
	}
}

func (s *ConfigService) SetDisableInMemoryCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemoryCache = disable
}

func (s *ConfigService) SetDownloadSignatureKeys(keys map[string][]byte) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloadSigKeys = make(map[string][]byte, len(keys))
	for kid, secret := range keys {
		kid = strings.TrimSpace(kid)
		if kid == "" || len(secret) == 0 {
			continue
		}
		cp := make([]byte, len(secret))
		copy(cp, secret)
		s.downloadSigKeys[kid] = cp
	}
}

func downloadSignaturePayload(e *rgsv1.DownloadLibraryEntry) string {
	if e == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(e.LibraryPath),
		strings.TrimSpace(e.Checksum),
		strings.TrimSpace(e.Version),
		e.Action.String(),
	}, "|")
}

func verifyDownloadSignature(e *rgsv1.DownloadLibraryEntry, secret []byte) bool {
	if e == nil || len(secret) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(downloadSignaturePayload(e)))
	expected := base64.RawStdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(e.Signature)))
}

func (s *ConfigService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *ConfigService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *ConfigService) authorize(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
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

func (s *ConfigService) nextChangeIDLocked() string {
	s.nextChangeID++
	return "cfg-" + strconv.FormatInt(s.nextChangeID, 10)
}

func (s *ConfigService) nextEntryIDLocked() string {
	s.nextEntryID++
	return "dll-" + strconv.FormatInt(s.nextEntryID, 10)
}

func (s *ConfigService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "config-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *ConfigService) appendAudit(meta *rgsv1.RequestMeta, objectType, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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

func cloneChange(c *rgsv1.ConfigChange) *rgsv1.ConfigChange {
	if c == nil {
		return nil
	}
	cp, _ := proto.Clone(c).(*rgsv1.ConfigChange)
	return cp
}

func cloneDownload(e *rgsv1.DownloadLibraryEntry) *rgsv1.DownloadLibraryEntry {
	if e == nil {
		return nil
	}
	cp, _ := proto.Clone(e).(*rgsv1.DownloadLibraryEntry)
	return cp
}

func keyFor(namespace, key string) string {
	return namespace + "::" + key
}

func (s *ConfigService) ProposeConfigChange(ctx context.Context, req *rgsv1.ProposeConfigChangeRequest) (*rgsv1.ProposeConfigChangeResponse, error) {
	if req == nil || req.ConfigNamespace == "" || req.ConfigKey == "" || req.ProposedValue == "" {
		return &rgsv1.ProposeConfigChangeResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "config_namespace, config_key and proposed_value are required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "config_change", "", "propose_config_change", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ProposeConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().Format(time.RFC3339Nano)
	id := s.nextChangeIDLocked()
	curr := s.currentValues[keyFor(req.ConfigNamespace, req.ConfigKey)]
	if s.db != nil {
		dbCurr, err := s.getCurrentValue(ctx, req.ConfigNamespace, req.ConfigKey)
		if err != nil {
			return &rgsv1.ProposeConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		curr = dbCurr
	}
	change := &rgsv1.ConfigChange{
		ChangeId:        id,
		ConfigNamespace: req.ConfigNamespace,
		ConfigKey:       req.ConfigKey,
		ProposedValue:   req.ProposedValue,
		PreviousValue:   curr,
		Reason:          req.Reason,
		Status:          rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_PROPOSED,
		ProposerId:      req.Meta.Actor.ActorId,
		CreatedAt:       now,
	}

	after, _ := json.Marshal(change)
	if err := s.appendAudit(req.Meta, "config_change", id, "propose_config_change", []byte(`{}`), after, audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.ProposeConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistConfigChange(ctx, change); err != nil {
		return &rgsv1.ProposeConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	if !s.disableInMemoryCache {
		s.changes[id] = change
		s.changeOrder = append(s.changeOrder, id)
	}
	return &rgsv1.ProposeConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Change: cloneChange(change)}, nil
}

func (s *ConfigService) ApproveConfigChange(ctx context.Context, req *rgsv1.ApproveConfigChangeRequest) (*rgsv1.ApproveConfigChangeResponse, error) {
	if req == nil || req.ChangeId == "" {
		return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "change_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "config_change", req.ChangeId, "approve_config_change", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	change := s.changes[req.ChangeId]
	if change == nil && s.db != nil {
		var err error
		change, err = s.getConfigChange(ctx, req.ChangeId)
		if err != nil {
			return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if change != nil && !s.disableInMemoryCache {
			s.changes[req.ChangeId] = change
		}
	}
	if change == nil {
		return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "change not found")}, nil
	}
	if change.Status != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_PROPOSED {
		return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "change is not in proposed state")}, nil
	}

	before, _ := json.Marshal(change)
	change.Status = rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPROVED
	change.ApproverId = req.Meta.Actor.ActorId
	change.ApprovedAt = s.now().Format(time.RFC3339Nano)
	after, _ := json.Marshal(change)
	if err := s.appendAudit(req.Meta, "config_change", change.ChangeId, "approve_config_change", before, after, audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistConfigChange(ctx, change); err != nil {
		return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	return &rgsv1.ApproveConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Change: cloneChange(change)}, nil
}

func (s *ConfigService) ApplyConfigChange(ctx context.Context, req *rgsv1.ApplyConfigChangeRequest) (*rgsv1.ApplyConfigChangeResponse, error) {
	if req == nil || req.ChangeId == "" {
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "change_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "config_change", req.ChangeId, "apply_config_change", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	change := s.changes[req.ChangeId]
	if change == nil && s.db != nil {
		var err error
		change, err = s.getConfigChange(ctx, req.ChangeId)
		if err != nil {
			return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if change != nil && !s.disableInMemoryCache {
			s.changes[req.ChangeId] = change
		}
	}
	if change == nil {
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "change not found")}, nil
	}
	if change.Status != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPROVED {
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "change is not approved")}, nil
	}

	before, _ := json.Marshal(change)
	change.Status = rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPLIED
	change.AppliedBy = req.Meta.Actor.ActorId
	change.AppliedAt = s.now().Format(time.RFC3339Nano)
	if !s.disableInMemoryCache {
		s.currentValues[keyFor(change.ConfigNamespace, change.ConfigKey)] = change.ProposedValue
	}
	after, _ := json.Marshal(change)
	if err := s.appendAudit(req.Meta, "config_change", change.ChangeId, "apply_config_change", before, after, audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistConfigChange(ctx, change); err != nil {
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.persistCurrentValue(ctx, change.ConfigNamespace, change.ConfigKey, change.ProposedValue, change.AppliedBy); err != nil {
		return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	return &rgsv1.ApplyConfigChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Change: cloneChange(change)}, nil
}

func (s *ConfigService) ListConfigHistory(ctx context.Context, req *rgsv1.ListConfigHistoryRequest) (*rgsv1.ListConfigHistoryResponse, error) {
	if req == nil {
		req = &rgsv1.ListConfigHistoryRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "config_change", "", "list_config_history", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ListConfigHistoryResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	start := 0
	if req.PageToken != "" {
		if p, err := strconv.Atoi(req.PageToken); err == nil && p >= 0 {
			start = p
		}
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 50
	}
	if s.db != nil {
		changes, err := s.listConfigHistoryFromDB(ctx, req.ConfigNamespaceFilter, size, start)
		if err != nil {
			return &rgsv1.ListConfigHistoryResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		next := ""
		if len(changes) == size {
			next = strconv.Itoa(start + len(changes))
		}
		return &rgsv1.ListConfigHistoryResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Changes: changes, NextPageToken: next}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	changes := make([]*rgsv1.ConfigChange, 0, len(s.changeOrder))
	for i := len(s.changeOrder) - 1; i >= 0; i-- {
		c := s.changes[s.changeOrder[i]]
		if c == nil {
			continue
		}
		if req.ConfigNamespaceFilter != "" && c.ConfigNamespace != req.ConfigNamespaceFilter {
			continue
		}
		changes = append(changes, cloneChange(c))
	}

	if start > len(changes) {
		start = len(changes)
	}
	end := start + size
	if end > len(changes) {
		end = len(changes)
	}
	next := ""
	if end < len(changes) {
		next = strconv.Itoa(end)
	}

	return &rgsv1.ListConfigHistoryResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Changes: changes[start:end], NextPageToken: next}, nil
}

func (s *ConfigService) RecordDownloadLibraryChange(ctx context.Context, req *rgsv1.RecordDownloadLibraryChangeRequest) (*rgsv1.RecordDownloadLibraryChangeResponse, error) {
	if req == nil || req.Entry == nil || req.Entry.LibraryPath == "" || req.Entry.Checksum == "" || req.Entry.Version == "" {
		return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "entry library_path/checksum/version are required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "download_library_entry", "", "record_download_library_change", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry := cloneDownload(req.Entry)
	if entry.EntryId == "" {
		entry.EntryId = s.nextEntryIDLocked()
	}
	if entry.ChangedBy == "" {
		entry.ChangedBy = req.Meta.Actor.ActorId
	}
	if entry.OccurredAt == "" {
		entry.OccurredAt = s.now().Format(time.RFC3339Nano)
	}
	if entry.Action == rgsv1.DownloadAction_DOWNLOAD_ACTION_ACTIVATE {
		if entry.SignerKid == "" || entry.Signature == "" {
			return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "activation requires signer_kid and signature")}, nil
		}
		secret := s.downloadSigKeys[entry.SignerKid]
		if !verifyDownloadSignature(entry, secret) {
			_ = s.appendAudit(req.Meta, "download_library_entry", entry.EntryId, "record_download_library_change", []byte(`{}`), []byte(`{}`), audit.ResultDenied, "invalid download signature")
			return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "invalid download signature")}, nil
		}
		if entry.SignatureAlg == "" {
			entry.SignatureAlg = "HMAC-SHA256"
		}
	}

	after, _ := json.Marshal(entry)
	if err := s.appendAudit(req.Meta, "download_library_entry", entry.EntryId, "record_download_library_change", []byte(`{}`), after, audit.ResultSuccess, entry.Reason); err != nil {
		return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistDownloadEntry(ctx, entry); err != nil {
		return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	if !s.disableInMemoryCache {
		s.downloadEntries[entry.EntryId] = entry
		s.downloadOrder = append(s.downloadOrder, entry.EntryId)
	}

	return &rgsv1.RecordDownloadLibraryChangeResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Entry: cloneDownload(entry)}, nil
}

func (s *ConfigService) ListDownloadLibraryChanges(ctx context.Context, req *rgsv1.ListDownloadLibraryChangesRequest) (*rgsv1.ListDownloadLibraryChangesResponse, error) {
	if req == nil {
		req = &rgsv1.ListDownloadLibraryChangesRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "download_library_entry", "", "list_download_library_changes", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ListDownloadLibraryChangesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	start := 0
	if req.PageToken != "" {
		if p, err := strconv.Atoi(req.PageToken); err == nil && p >= 0 {
			start = p
		}
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 50
	}
	if s.db != nil {
		entries, err := s.listDownloadEntriesFromDB(ctx, size, start)
		if err != nil {
			return &rgsv1.ListDownloadLibraryChangesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		next := ""
		if len(entries) == size {
			next = strconv.Itoa(start + len(entries))
		}
		return &rgsv1.ListDownloadLibraryChangesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Entries: entries, NextPageToken: next}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]*rgsv1.DownloadLibraryEntry, 0, len(s.downloadOrder))
	for _, id := range s.downloadOrder {
		entries = append(entries, cloneDownload(s.downloadEntries[id]))
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].OccurredAt == entries[j].OccurredAt {
			return entries[i].EntryId < entries[j].EntryId
		}
		return entries[i].OccurredAt > entries[j].OccurredAt
	})

	if start > len(entries) {
		start = len(entries)
	}
	end := start + size
	if end > len(entries) {
		end = len(entries)
	}
	next := ""
	if end < len(entries) {
		next = strconv.Itoa(end)
	}

	return &rgsv1.ListDownloadLibraryChangesResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Entries: entries[start:end], NextPageToken: next}, nil
}
