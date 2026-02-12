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

type ledgerAccount struct {
	id        string
	currency  string
	available int64
	pending   int64
}

type ledgerPosting struct {
	accountID string
	direction string
	amount    int64
	currency  string
	createdAt time.Time
}

type LedgerService struct {
	rgsv1.UnimplementedLedgerServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu sync.Mutex

	accounts               map[string]*ledgerAccount
	transactionsByAcct     map[string][]*rgsv1.LedgerTransaction
	postingsByTx           map[string][]ledgerPosting
	depositByIdempotency   map[string]*rgsv1.DepositResponse
	withdrawByIdempotency  map[string]*rgsv1.WithdrawResponse
	toDeviceByIdempotency  map[string]*rgsv1.TransferToDeviceResponse
	toAccountByIdempotency map[string]*rgsv1.TransferToAccountResponse
	nextTransactionID      int64
	nextTransferID         int64
	nextAuditID            int64
	eftFraudFailures       map[string]int
	eftFraudLockedUntil    map[string]time.Time
	eftFraudMaxFailures    int
	eftFraudLockoutTTL     time.Duration
	db                     *sql.DB
	idempotencyTTL         time.Duration
	disableInMemIdemCache  bool
}

func NewLedgerService(clk clock.Clock, db ...*sql.DB) *LedgerService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &LedgerService{
		Clock:                  clk,
		AuditStore:             audit.NewInMemoryStore(),
		accounts:               make(map[string]*ledgerAccount),
		transactionsByAcct:     make(map[string][]*rgsv1.LedgerTransaction),
		postingsByTx:           make(map[string][]ledgerPosting),
		depositByIdempotency:   make(map[string]*rgsv1.DepositResponse),
		withdrawByIdempotency:  make(map[string]*rgsv1.WithdrawResponse),
		toDeviceByIdempotency:  make(map[string]*rgsv1.TransferToDeviceResponse),
		toAccountByIdempotency: make(map[string]*rgsv1.TransferToAccountResponse),
		eftFraudFailures:       make(map[string]int),
		eftFraudLockedUntil:    make(map[string]time.Time),
		eftFraudMaxFailures:    5,
		eftFraudLockoutTTL:     15 * time.Minute,
		db:                     handle,
		idempotencyTTL:         24 * time.Hour,
	}
}

func (s *LedgerService) SetIdempotencyTTL(ttl time.Duration) {
	if s == nil {
		return
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idempotencyTTL = ttl
}

func (s *LedgerService) getIdempotencyTTL() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idempotencyTTL <= 0 {
		return 24 * time.Hour
	}
	return s.idempotencyTTL
}

func (s *LedgerService) SetEFTFraudPolicy(maxFailures int, ttl time.Duration) {
	if s == nil {
		return
	}
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eftFraudMaxFailures = maxFailures
	s.eftFraudLockoutTTL = ttl
}

func (s *LedgerService) SetDisableInMemoryIdempotencyCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemIdemCache = disable
}

func (s *LedgerService) useInMemoryIdempotencyCache() bool {
	if s == nil {
		return false
	}
	if s.dbEnabled() && s.disableInMemIdemCache {
		return false
	}
	return true
}

func (s *LedgerService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func requestID(meta *rgsv1.RequestMeta) string {
	if meta == nil {
		return ""
	}
	return meta.RequestId
}

func idempotency(meta *rgsv1.RequestMeta) string {
	if meta == nil {
		return ""
	}
	return meta.IdempotencyKey
}

func (s *LedgerService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func invalidAmount(m *rgsv1.Money) bool {
	if m == nil {
		return true
	}
	if m.AmountMinor <= 0 {
		return true
	}
	return m.Currency == ""
}

func money(amount int64, currency string) *rgsv1.Money {
	return &rgsv1.Money{AmountMinor: amount, Currency: currency}
}

func (s *LedgerService) accountBalance(accountID string) (available int64, pending int64, currency string, ok bool) {
	acct, found := s.accounts[accountID]
	if !found {
		return 0, 0, "", false
	}
	return acct.available, acct.pending, acct.currency, true
}

func (s *LedgerService) getOrCreateAccount(accountID string, currency string) *ledgerAccount {
	if acct, ok := s.accounts[accountID]; ok {
		return acct
	}
	acct := &ledgerAccount{id: accountID, currency: currency}
	s.accounts[accountID] = acct
	return acct
}

func transactionCopy(in *rgsv1.LedgerTransaction) *rgsv1.LedgerTransaction {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.LedgerTransaction)
	return cp
}

func (s *LedgerService) appendTransaction(tx *rgsv1.LedgerTransaction) {
	s.transactionsByAcct[tx.AccountId] = append(s.transactionsByAcct[tx.AccountId], transactionCopy(tx))
}

func (s *LedgerService) rollbackLastTransaction(accountID string) {
	txs := s.transactionsByAcct[accountID]
	if len(txs) == 0 {
		return
	}
	s.transactionsByAcct[accountID] = txs[:len(txs)-1]
}

func (s *LedgerService) nextTxIDLocked() string {
	s.nextTransactionID++
	return "tx-" + strconv.FormatInt(s.nextTransactionID, 10)
}

func (s *LedgerService) nextTransferIDLocked() string {
	s.nextTransferID++
	return "tr-" + strconv.FormatInt(s.nextTransferID, 10)
}

func (s *LedgerService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *LedgerService) addPostings(txID string, postings []ledgerPosting) bool {
	if !isBalanced(postings) {
		return false
	}
	s.postingsByTx[txID] = append(s.postingsByTx[txID], postings...)
	return true
}

func isBalanced(postings []ledgerPosting) bool {
	var total int64
	for _, p := range postings {
		switch p.direction {
		case "credit":
			total += p.amount
		case "debit":
			total -= p.amount
		default:
			return false
		}
	}
	return total == 0
}

func (s *LedgerService) authorize(ctx context.Context, meta *rgsv1.RequestMeta, accountID string) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	switch actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	case rgsv1.ActorType_ACTOR_TYPE_PLAYER:
		if accountID != actor.ActorId {
			return false, "player cannot access another account"
		}
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func snapshotAccount(acct *ledgerAccount) []byte {
	if acct == nil {
		return []byte(`{}`)
	}
	payload := map[string]any{
		"account_id": acct.id,
		"currency":   acct.currency,
		"available":  acct.available,
		"pending":    acct.pending,
	}
	b, _ := json.Marshal(payload)
	return b
}

func (s *LedgerService) appendAudit(meta *rgsv1.RequestMeta, objectType, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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

func (s *LedgerService) auditDenied(meta *rgsv1.RequestMeta, objectType, objectID, action, reason string) {
	_ = s.appendAudit(meta, objectType, objectID, action, []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
}

func (s *LedgerService) eftLocked(accountID string) bool {
	return s.eftFraudLockedUntil[accountID].After(s.now())
}

func (s *LedgerService) recordEFTFailure(accountID string) {
	if accountID == "" {
		return
	}
	s.eftFraudFailures[accountID]++
	if s.eftFraudFailures[accountID] >= s.eftFraudMaxFailures {
		s.eftFraudLockedUntil[accountID] = s.now().Add(s.eftFraudLockoutTTL)
	}
}

func (s *LedgerService) resetEFTFailures(accountID string) {
	if accountID == "" {
		return
	}
	delete(s.eftFraudFailures, accountID)
	delete(s.eftFraudLockedUntil, accountID)
}

func (s *LedgerService) AuditEvents() []audit.Event {
	if s.AuditStore == nil {
		return nil
	}
	return s.AuditStore.Events()
}

func (s *LedgerService) GetBalance(ctx context.Context, req *rgsv1.GetBalanceRequest) (*rgsv1.GetBalanceResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.GetBalanceResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta, req.AccountId); !ok {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "get_balance", reason)
		return &rgsv1.GetBalanceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	available, pending, currency, ok := s.accountBalance(req.AccountId)
	if s.dbEnabled() {
		dbAvailable, dbPending, dbCurrency, dbOK, err := s.getBalanceFromDB(ctx, req.AccountId)
		if err != nil {
			return &rgsv1.GetBalanceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if dbOK {
			available, pending, currency, ok = dbAvailable, dbPending, dbCurrency, true
		}
	}
	if !ok {
		currency = "USD"
	}

	return &rgsv1.GetBalanceResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		AccountId:        req.AccountId,
		AvailableBalance: money(available, currency),
		PendingBalance:   money(pending, currency),
	}, nil
}

func (s *LedgerService) Deposit(ctx context.Context, req *rgsv1.DepositRequest) (*rgsv1.DepositResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta, req.AccountId); !ok {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "deposit", reason)
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if invalidAmount(req.Amount) {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "amount must be > 0 and currency provided")}, nil
	}
	idem := idempotency(req.Meta)
	if idem == "" {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eftLocked(req.AccountId) {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "deposit", "eft account locked")
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "eft account locked")}, nil
	}

	key := req.AccountId + "|deposit|" + idem
	scope := idemScope(req.AccountId, "deposit")
	requestHash := hashRequest(scope, req.Amount.GetCurrency(), strconv.FormatInt(req.Amount.GetAmountMinor(), 10), req.AuthorizationId)
	if s.useInMemoryIdempotencyCache() {
		if prev, ok := s.depositByIdempotency[key]; ok {
			cp, _ := proto.Clone(prev).(*rgsv1.DepositResponse)
			return cp, nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.DepositResponse
		found, err := s.loadIdempotencyResponse(ctx, scope, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryIdempotencyCache() {
				s.depositByIdempotency[key], _ = proto.Clone(&replay).(*rgsv1.DepositResponse)
			}
			return &replay, nil
		}
	}
	if s.dbEnabled() {
		tx, found, err := s.findTransactionByIdempotency(ctx, req.AccountId, rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_DEPOSIT, idem)
		if err != nil {
			return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			available, _, currency, ok, balErr := s.getBalanceFromDB(ctx, req.AccountId)
			if balErr != nil {
				return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
			}
			if !ok {
				currency = req.Amount.Currency
			}
			resp := &rgsv1.DepositResponse{
				Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
				Transaction:      tx,
				AvailableBalance: money(available, currency),
			}
			if s.useInMemoryIdempotencyCache() {
				s.depositByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.DepositResponse)
			}
			return resp, nil
		}
	}

	acct := s.getOrCreateAccount(req.AccountId, req.Amount.Currency)
	if acct.currency != req.Amount.Currency {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}

	before := snapshotAccount(acct)
	now := s.now()
	txID := s.nextTxIDLocked()
	postings := []ledgerPosting{
		{accountID: "operator_liability", direction: "debit", amount: req.Amount.AmountMinor, currency: req.Amount.Currency, createdAt: now},
		{accountID: req.AccountId, direction: "credit", amount: req.Amount.AmountMinor, currency: req.Amount.Currency, createdAt: now},
	}
	if !s.addPostings(txID, postings) {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "unbalanced postings")}, nil
	}

	acct.available += req.Amount.AmountMinor
	tx := &rgsv1.LedgerTransaction{
		TransactionId:   txID,
		AccountId:       req.AccountId,
		TransactionType: rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_DEPOSIT,
		Amount:          money(req.Amount.AmountMinor, req.Amount.Currency),
		OccurredAt:      now.Format(time.RFC3339Nano),
		AuthorizationId: req.AuthorizationId,
		Description:     "deposit accepted",
	}
	s.appendTransaction(tx)

	after := snapshotAccount(acct)
	if err := s.appendAudit(req.Meta, "ledger_account", req.AccountId, "deposit", before, after, audit.ResultSuccess, ""); err != nil {
		acct.available -= req.Amount.AmountMinor
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistLedgerMutation(ctx, tx, postings, "accepted", idem); err != nil {
		acct.available -= req.Amount.AmountMinor
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	resp := &rgsv1.DepositResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transaction:      tx,
		AvailableBalance: money(acct.available, acct.currency),
	}
	if err := s.persistIdempotencyResponse(ctx, scope, idem, requestHash, resp.Meta.GetResultCode(), resp); err != nil {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if s.useInMemoryIdempotencyCache() {
		s.depositByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.DepositResponse)
	}
	s.resetEFTFailures(req.AccountId)
	return resp, nil
}

func (s *LedgerService) Withdraw(ctx context.Context, req *rgsv1.WithdrawRequest) (*rgsv1.WithdrawResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta, req.AccountId); !ok {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "withdraw", reason)
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if invalidAmount(req.Amount) {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "amount must be > 0 and currency provided")}, nil
	}
	idem := idempotency(req.Meta)
	if idem == "" {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eftLocked(req.AccountId) {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "withdraw", "eft account locked")
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "eft account locked")}, nil
	}

	key := req.AccountId + "|withdraw|" + idem
	scope := idemScope(req.AccountId, "withdraw")
	requestHash := hashRequest(scope, req.Amount.GetCurrency(), strconv.FormatInt(req.Amount.GetAmountMinor(), 10))
	if s.useInMemoryIdempotencyCache() {
		if prev, ok := s.withdrawByIdempotency[key]; ok {
			cp, _ := proto.Clone(prev).(*rgsv1.WithdrawResponse)
			return cp, nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.WithdrawResponse
		found, err := s.loadIdempotencyResponse(ctx, scope, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryIdempotencyCache() {
				s.withdrawByIdempotency[key], _ = proto.Clone(&replay).(*rgsv1.WithdrawResponse)
			}
			return &replay, nil
		}
	}
	if s.dbEnabled() {
		tx, found, err := s.findTransactionByIdempotency(ctx, req.AccountId, rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_WITHDRAWAL, idem)
		if err != nil {
			return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			available, _, currency, ok, balErr := s.getBalanceFromDB(ctx, req.AccountId)
			if balErr != nil {
				return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
			}
			if !ok {
				currency = req.Amount.Currency
			}
			resp := &rgsv1.WithdrawResponse{
				Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
				Transaction:      tx,
				AvailableBalance: money(available, currency),
			}
			if s.useInMemoryIdempotencyCache() {
				s.withdrawByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.WithdrawResponse)
			}
			return resp, nil
		}
	}

	acct := s.getOrCreateAccount(req.AccountId, req.Amount.Currency)
	if acct.currency != req.Amount.Currency {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}
	if acct.available < req.Amount.AmountMinor {
		s.recordEFTFailure(req.AccountId)
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "withdraw", "insufficient balance")
		resp := &rgsv1.WithdrawResponse{
			Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "insufficient balance"),
			AvailableBalance: money(acct.available, acct.currency),
		}
		if err := s.persistIdempotencyResponse(ctx, scope, idem, requestHash, resp.Meta.GetResultCode(), resp); err != nil {
			return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if s.useInMemoryIdempotencyCache() {
			s.withdrawByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.WithdrawResponse)
		}
		return resp, nil
	}

	before := snapshotAccount(acct)
	now := s.now()
	txID := s.nextTxIDLocked()
	postings := []ledgerPosting{
		{accountID: req.AccountId, direction: "debit", amount: req.Amount.AmountMinor, currency: req.Amount.Currency, createdAt: now},
		{accountID: "operator_liability", direction: "credit", amount: req.Amount.AmountMinor, currency: req.Amount.Currency, createdAt: now},
	}
	if !s.addPostings(txID, postings) {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "unbalanced postings")}, nil
	}

	acct.available -= req.Amount.AmountMinor
	tx := &rgsv1.LedgerTransaction{
		TransactionId:   txID,
		AccountId:       req.AccountId,
		TransactionType: rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_WITHDRAWAL,
		Amount:          money(req.Amount.AmountMinor, req.Amount.Currency),
		OccurredAt:      now.Format(time.RFC3339Nano),
		Description:     "withdrawal accepted",
	}
	s.appendTransaction(tx)

	after := snapshotAccount(acct)
	if err := s.appendAudit(req.Meta, "ledger_account", req.AccountId, "withdraw", before, after, audit.ResultSuccess, ""); err != nil {
		acct.available += req.Amount.AmountMinor
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistLedgerMutation(ctx, tx, postings, "accepted", idem); err != nil {
		acct.available += req.Amount.AmountMinor
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	resp := &rgsv1.WithdrawResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transaction:      tx,
		AvailableBalance: money(acct.available, acct.currency),
	}
	if err := s.persistIdempotencyResponse(ctx, scope, idem, requestHash, resp.Meta.GetResultCode(), resp); err != nil {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if s.useInMemoryIdempotencyCache() {
		s.withdrawByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.WithdrawResponse)
	}
	s.resetEFTFailures(req.AccountId)
	return resp, nil
}

func (s *LedgerService) TransferToDevice(ctx context.Context, req *rgsv1.TransferToDeviceRequest) (*rgsv1.TransferToDeviceResponse, error) {
	if req == nil || req.AccountId == "" || req.DeviceId == "" {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id and device_id are required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta, req.AccountId); !ok {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "transfer_to_device", reason)
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if invalidAmount(req.RequestedAmount) {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "requested_amount must be > 0 and currency provided")}, nil
	}
	idem := idempotency(req.Meta)
	if idem == "" {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eftLocked(req.AccountId) {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "transfer_to_device", "eft account locked")
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "eft account locked")}, nil
	}

	key := req.AccountId + "|to_device|" + idem
	scope := idemScope(req.AccountId, "transfer_to_device")
	requestHash := hashRequest(scope, req.DeviceId, req.RequestedAmount.GetCurrency(), strconv.FormatInt(req.RequestedAmount.GetAmountMinor(), 10))
	if s.useInMemoryIdempotencyCache() {
		if prev, ok := s.toDeviceByIdempotency[key]; ok {
			cp, _ := proto.Clone(prev).(*rgsv1.TransferToDeviceResponse)
			return cp, nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.TransferToDeviceResponse
		found, err := s.loadIdempotencyResponse(ctx, scope, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryIdempotencyCache() {
				s.toDeviceByIdempotency[key], _ = proto.Clone(&replay).(*rgsv1.TransferToDeviceResponse)
			}
			return &replay, nil
		}
	}

	acct := s.getOrCreateAccount(req.AccountId, req.RequestedAmount.Currency)
	if acct.currency != req.RequestedAmount.Currency {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}

	if acct.available <= 0 {
		s.recordEFTFailure(req.AccountId)
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "transfer_to_device", "insufficient balance")
		resp := &rgsv1.TransferToDeviceResponse{
			Meta:              s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "insufficient balance"),
			TransferStatus:    rgsv1.TransferStatus_TRANSFER_STATUS_DENIED,
			TransferredAmount: money(0, acct.currency),
			AvailableBalance:  money(acct.available, acct.currency),
		}
		if err := s.persistIdempotencyResponse(ctx, scope, idem, requestHash, resp.Meta.GetResultCode(), resp); err != nil {
			return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if s.useInMemoryIdempotencyCache() {
			s.toDeviceByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.TransferToDeviceResponse)
		}
		return resp, nil
	}

	transfer := req.RequestedAmount.AmountMinor
	status := rgsv1.TransferStatus_TRANSFER_STATUS_ACCEPTED
	reason := ""
	if acct.available < transfer {
		transfer = acct.available
		status = rgsv1.TransferStatus_TRANSFER_STATUS_PARTIAL
		reason = "requested amount exceeds available balance"
	}

	before := snapshotAccount(acct)
	now := s.now()
	txID := s.nextTxIDLocked()
	postings := []ledgerPosting{
		{accountID: req.AccountId, direction: "debit", amount: transfer, currency: req.RequestedAmount.Currency, createdAt: now},
		{accountID: "device_escrow:" + req.DeviceId, direction: "credit", amount: transfer, currency: req.RequestedAmount.Currency, createdAt: now},
	}
	if !s.addPostings(txID, postings) {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "unbalanced postings")}, nil
	}
	acct.available -= transfer

	tx := &rgsv1.LedgerTransaction{
		TransactionId:   txID,
		AccountId:       req.AccountId,
		TransactionType: rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_DEVICE,
		Amount:          money(transfer, req.RequestedAmount.Currency),
		OccurredAt:      now.Format(time.RFC3339Nano),
		Description:     "transfer to device",
	}
	s.appendTransaction(tx)

	after := snapshotAccount(acct)
	if err := s.appendAudit(req.Meta, "ledger_account", req.AccountId, "transfer_to_device", before, after, audit.ResultSuccess, reason); err != nil {
		acct.available += transfer
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistLedgerMutation(ctx, tx, postings, "accepted", idem); err != nil {
		acct.available += transfer
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	resp := &rgsv1.TransferToDeviceResponse{
		Meta:              s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		TransferId:        s.nextTransferIDLocked(),
		TransferStatus:    status,
		TransferredAmount: money(transfer, acct.currency),
		AvailableBalance:  money(acct.available, acct.currency),
		UnresolvedReason:  reason,
	}
	if err := s.persistIdempotencyResponse(ctx, scope, idem, requestHash, resp.Meta.GetResultCode(), resp); err != nil {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if s.useInMemoryIdempotencyCache() {
		s.toDeviceByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.TransferToDeviceResponse)
	}
	s.resetEFTFailures(req.AccountId)
	return resp, nil
}

func (s *LedgerService) TransferToAccount(ctx context.Context, req *rgsv1.TransferToAccountRequest) (*rgsv1.TransferToAccountResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta, req.AccountId); !ok {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "transfer_to_account", reason)
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if invalidAmount(req.Amount) {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "amount must be > 0 and currency provided")}, nil
	}
	idem := idempotency(req.Meta)
	if idem == "" {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eftLocked(req.AccountId) {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "transfer_to_account", "eft account locked")
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "eft account locked")}, nil
	}

	key := req.AccountId + "|to_account|" + idem
	scope := idemScope(req.AccountId, "transfer_to_account")
	requestHash := hashRequest(scope, req.Amount.GetCurrency(), strconv.FormatInt(req.Amount.GetAmountMinor(), 10))
	if s.useInMemoryIdempotencyCache() {
		if prev, ok := s.toAccountByIdempotency[key]; ok {
			cp, _ := proto.Clone(prev).(*rgsv1.TransferToAccountResponse)
			return cp, nil
		}
	}
	if s.dbEnabled() {
		var replay rgsv1.TransferToAccountResponse
		found, err := s.loadIdempotencyResponse(ctx, scope, idem, requestHash, &replay)
		if err == errIdempotencyRequestMismatch {
			return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "idempotency_key reused with different request")}, nil
		}
		if err != nil {
			return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			if s.useInMemoryIdempotencyCache() {
				s.toAccountByIdempotency[key], _ = proto.Clone(&replay).(*rgsv1.TransferToAccountResponse)
			}
			return &replay, nil
		}
	}
	if s.dbEnabled() {
		tx, found, err := s.findTransactionByIdempotency(ctx, req.AccountId, rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_ACCOUNT, idem)
		if err != nil {
			return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if found {
			available, _, currency, ok, balErr := s.getBalanceFromDB(ctx, req.AccountId)
			if balErr != nil {
				return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
			}
			if !ok {
				currency = req.Amount.Currency
			}
			resp := &rgsv1.TransferToAccountResponse{
				Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
				Transaction:      tx,
				AvailableBalance: money(available, currency),
			}
			if s.useInMemoryIdempotencyCache() {
				s.toAccountByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.TransferToAccountResponse)
			}
			return resp, nil
		}
	}

	acct := s.getOrCreateAccount(req.AccountId, req.Amount.Currency)
	if acct.currency != req.Amount.Currency {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}

	before := snapshotAccount(acct)
	now := s.now()
	txID := s.nextTxIDLocked()
	postings := []ledgerPosting{
		{accountID: "device_escrow", direction: "debit", amount: req.Amount.AmountMinor, currency: req.Amount.Currency, createdAt: now},
		{accountID: req.AccountId, direction: "credit", amount: req.Amount.AmountMinor, currency: req.Amount.Currency, createdAt: now},
	}
	if !s.addPostings(txID, postings) {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "unbalanced postings")}, nil
	}
	acct.available += req.Amount.AmountMinor

	tx := &rgsv1.LedgerTransaction{
		TransactionId:   txID,
		AccountId:       req.AccountId,
		TransactionType: rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_ACCOUNT,
		Amount:          money(req.Amount.AmountMinor, req.Amount.Currency),
		OccurredAt:      now.Format(time.RFC3339Nano),
		Description:     "transfer to account",
	}
	s.appendTransaction(tx)

	after := snapshotAccount(acct)
	if err := s.appendAudit(req.Meta, "ledger_account", req.AccountId, "transfer_to_account", before, after, audit.ResultSuccess, ""); err != nil {
		acct.available -= req.Amount.AmountMinor
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistLedgerMutation(ctx, tx, postings, "accepted", idem); err != nil {
		acct.available -= req.Amount.AmountMinor
		delete(s.postingsByTx, txID)
		s.rollbackLastTransaction(req.AccountId)
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	resp := &rgsv1.TransferToAccountResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transaction:      tx,
		AvailableBalance: money(acct.available, acct.currency),
	}
	if err := s.persistIdempotencyResponse(ctx, scope, idem, requestHash, resp.Meta.GetResultCode(), resp); err != nil {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if s.useInMemoryIdempotencyCache() {
		s.toAccountByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.TransferToAccountResponse)
	}
	s.resetEFTFailures(req.AccountId)
	return resp, nil
}

func (s *LedgerService) ListTransactions(ctx context.Context, req *rgsv1.ListTransactionsRequest) (*rgsv1.ListTransactionsResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.ListTransactionsResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta, req.AccountId); !ok {
		s.auditDenied(req.Meta, "ledger_account", req.AccountId, "list_transactions", reason)
		return &rgsv1.ListTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	txs := s.transactionsByAcct[req.AccountId]
	if s.dbEnabled() {
		start := 0
		if req.PageToken != "" {
			if parsed, err := strconv.Atoi(req.PageToken); err == nil && parsed >= 0 {
				start = parsed
			}
		}
		pageSize := int(req.PageSize)
		if pageSize <= 0 {
			pageSize = 50
		}
		dbTxs, err := s.listTransactionsFromDB(ctx, req.AccountId, pageSize, start)
		if err != nil {
			return &rgsv1.ListTransactionsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if dbTxs != nil {
			nextToken := ""
			if len(dbTxs) == pageSize {
				nextToken = strconv.Itoa(start + len(dbTxs))
			}
			return &rgsv1.ListTransactionsResponse{
				Meta:          s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
				Transactions:  dbTxs,
				NextPageToken: nextToken,
			}, nil
		}
	}
	start := 0
	if req.PageToken != "" {
		if parsed, err := strconv.Atoi(req.PageToken); err == nil && parsed >= 0 {
			start = parsed
		}
	}
	if start > len(txs) {
		start = len(txs)
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	end := start + pageSize
	if end > len(txs) {
		end = len(txs)
	}

	items := make([]*rgsv1.LedgerTransaction, 0, end-start)
	for _, tx := range txs[start:end] {
		items = append(items, transactionCopy(tx))
	}

	nextToken := ""
	if end < len(txs) {
		nextToken = strconv.Itoa(end)
	}

	return &rgsv1.ListTransactionsResponse{
		Meta:          s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transactions:  items,
		NextPageToken: nextToken,
	}, nil
}
