package models

import (
	"time"

	"github.com/uptrace/bun"
)

// OFBizUserLoginDBModel is the database model for the OFBiz USER_LOGIN table
// Maps to Apache OFBiz standard UserLogin entity
type OFBizUserLoginPasswordHistoryDBModel struct {
	bun.BaseModel      `bun:"table:user_login_password_history,alias:ul"`
	UserLoginID        string     `bun:"user_login_id,pk,type:varchar(255)"`
	VisitID            string     `bun:"visit_id,type:varchar(20)"`
	FromDate           time.Time  `bun:"from_date,pk"`
	ThruDate           *time.Time `bun:"thru_date"`
	PasswordUsed       string     `bun:"password_used,type:varchar(255)"`
	SuccessfulLogin    string     `bun:"successful_login,type:char(1)"`
	OriginUserLoginID  string     `bun:"origin_user_login_id,type:varchar(255)"`
	LastUpdatedStamp   time.Time  `bun:"last_updated_stamp"`
	LastUpdatedTxStamp time.Time  `bun:"last_updated_tx_stamp"`
	CreatedStamp       time.Time  `bun:"created_stamp"`
	CreatedTxStamp     time.Time  `bun:"created_tx_stamp"`
	PartyID            string     `bun:"party_id,type:varchar(20)"`
}
