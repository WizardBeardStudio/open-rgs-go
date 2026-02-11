package audit

import "time"

type Result string

const (
	ResultSuccess Result = "success"
	ResultDenied  Result = "denied"
	ResultError   Result = "error"
)

type Event struct {
	AuditID      string
	OccurredAt   time.Time
	RecordedAt   time.Time
	ActorID      string
	ActorType    string
	AuthContext  string
	ObjectType   string
	ObjectID     string
	Action       string
	Before       []byte
	After        []byte
	Result       Result
	Reason       string
	PartitionDay string
	HashPrev     string
	HashCurr     string
}
