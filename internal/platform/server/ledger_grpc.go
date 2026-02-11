package server

import (
	"context"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
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

	Clock clock.Clock

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
}

func NewLedgerService(clk clock.Clock) *LedgerService {
	return &LedgerService{
		Clock:                  clk,
		accounts:               make(map[string]*ledgerAccount),
		transactionsByAcct:     make(map[string][]*rgsv1.LedgerTransaction),
		postingsByTx:           make(map[string][]ledgerPosting),
		depositByIdempotency:   make(map[string]*rgsv1.DepositResponse),
		withdrawByIdempotency:  make(map[string]*rgsv1.WithdrawResponse),
		toDeviceByIdempotency:  make(map[string]*rgsv1.TransferToDeviceResponse),
		toAccountByIdempotency: make(map[string]*rgsv1.TransferToAccountResponse),
	}
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

func (s *LedgerService) nextTxIDLocked() string {
	s.nextTransactionID++
	return "tx-" + strconv.FormatInt(s.nextTransactionID, 10)
}

func (s *LedgerService) nextTransferIDLocked() string {
	s.nextTransferID++
	return "tr-" + strconv.FormatInt(s.nextTransferID, 10)
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

func (s *LedgerService) GetBalance(_ context.Context, req *rgsv1.GetBalanceRequest) (*rgsv1.GetBalanceResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.GetBalanceResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	available, pending, currency, ok := s.accountBalance(req.AccountId)
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

func (s *LedgerService) Deposit(_ context.Context, req *rgsv1.DepositRequest) (*rgsv1.DepositResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
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

	key := req.AccountId + "|deposit|" + idem
	if prev, ok := s.depositByIdempotency[key]; ok {
		cp, _ := proto.Clone(prev).(*rgsv1.DepositResponse)
		return cp, nil
	}

	acct := s.getOrCreateAccount(req.AccountId, req.Amount.Currency)
	if acct.currency != req.Amount.Currency {
		return &rgsv1.DepositResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}

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

	resp := &rgsv1.DepositResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transaction:      tx,
		AvailableBalance: money(acct.available, acct.currency),
	}
	s.depositByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.DepositResponse)
	return resp, nil
}

func (s *LedgerService) Withdraw(_ context.Context, req *rgsv1.WithdrawRequest) (*rgsv1.WithdrawResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
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

	key := req.AccountId + "|withdraw|" + idem
	if prev, ok := s.withdrawByIdempotency[key]; ok {
		cp, _ := proto.Clone(prev).(*rgsv1.WithdrawResponse)
		return cp, nil
	}

	acct := s.getOrCreateAccount(req.AccountId, req.Amount.Currency)
	if acct.currency != req.Amount.Currency {
		return &rgsv1.WithdrawResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}
	if acct.available < req.Amount.AmountMinor {
		return &rgsv1.WithdrawResponse{
			Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "insufficient balance"),
			AvailableBalance: money(acct.available, acct.currency),
		}, nil
	}

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

	resp := &rgsv1.WithdrawResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transaction:      tx,
		AvailableBalance: money(acct.available, acct.currency),
	}
	s.withdrawByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.WithdrawResponse)
	return resp, nil
}

func (s *LedgerService) TransferToDevice(_ context.Context, req *rgsv1.TransferToDeviceRequest) (*rgsv1.TransferToDeviceResponse, error) {
	if req == nil || req.AccountId == "" || req.DeviceId == "" {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id and device_id are required")}, nil
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

	key := req.AccountId + "|to_device|" + idem
	if prev, ok := s.toDeviceByIdempotency[key]; ok {
		cp, _ := proto.Clone(prev).(*rgsv1.TransferToDeviceResponse)
		return cp, nil
	}

	acct := s.getOrCreateAccount(req.AccountId, req.RequestedAmount.Currency)
	if acct.currency != req.RequestedAmount.Currency {
		return &rgsv1.TransferToDeviceResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}

	if acct.available <= 0 {
		return &rgsv1.TransferToDeviceResponse{
			Meta:              s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "insufficient balance"),
			TransferStatus:    rgsv1.TransferStatus_TRANSFER_STATUS_DENIED,
			TransferredAmount: money(0, acct.currency),
			AvailableBalance:  money(acct.available, acct.currency),
		}, nil
	}

	transfer := req.RequestedAmount.AmountMinor
	status := rgsv1.TransferStatus_TRANSFER_STATUS_ACCEPTED
	reason := ""
	if acct.available < transfer {
		transfer = acct.available
		status = rgsv1.TransferStatus_TRANSFER_STATUS_PARTIAL
		reason = "requested amount exceeds available balance"
	}

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

	resp := &rgsv1.TransferToDeviceResponse{
		Meta:              s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		TransferId:        s.nextTransferIDLocked(),
		TransferStatus:    status,
		TransferredAmount: money(transfer, acct.currency),
		AvailableBalance:  money(acct.available, acct.currency),
		UnresolvedReason:  reason,
	}
	s.toDeviceByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.TransferToDeviceResponse)
	return resp, nil
}

func (s *LedgerService) TransferToAccount(_ context.Context, req *rgsv1.TransferToAccountRequest) (*rgsv1.TransferToAccountResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
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

	key := req.AccountId + "|to_account|" + idem
	if prev, ok := s.toAccountByIdempotency[key]; ok {
		cp, _ := proto.Clone(prev).(*rgsv1.TransferToAccountResponse)
		return cp, nil
	}

	acct := s.getOrCreateAccount(req.AccountId, req.Amount.Currency)
	if acct.currency != req.Amount.Currency {
		return &rgsv1.TransferToAccountResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "currency mismatch for account")}, nil
	}

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

	resp := &rgsv1.TransferToAccountResponse{
		Meta:             s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Transaction:      tx,
		AvailableBalance: money(acct.available, acct.currency),
	}
	s.toAccountByIdempotency[key], _ = proto.Clone(resp).(*rgsv1.TransferToAccountResponse)
	return resp, nil
}

func (s *LedgerService) ListTransactions(_ context.Context, req *rgsv1.ListTransactionsRequest) (*rgsv1.ListTransactionsResponse, error) {
	if req == nil || req.AccountId == "" {
		return &rgsv1.ListTransactionsResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "account_id is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	txs := s.transactionsByAcct[req.AccountId]
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
