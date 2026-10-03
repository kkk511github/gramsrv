package domain

// MaxDeletePhoneCallHistoryBatch fits the existing private-history query limit.
const MaxDeletePhoneCallHistoryBatch = 500

type DeletePhoneCallHistoryRequest struct {
	Revoke          bool
	Date            int
	OriginAuthKeyID [8]byte
	OriginSessionID int64
}
