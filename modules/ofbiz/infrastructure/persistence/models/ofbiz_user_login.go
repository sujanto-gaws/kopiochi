package models

import (
	"time"

	"github.com/uptrace/bun"
)

// OFBizUserLoginDBModel is the database model for the OFBiz USER_LOGIN table
// Maps to Apache OFBiz standard UserLogin entity
type OFBizUserLoginDBModel struct {
	bun.BaseModel          `bun:"table:user_login,alias:ul"`
	UserLoginID            string     `bun:"user_login_id,pk,type:varchar(255)"`
	CurrentPassword        string     `bun:"current_password,type:varchar(255)"`
	PasswordHint           string     `bun:"password_hint,type:varchar(255)"`
	IsSystem               string     `bun:"is_system,type:char(1)"`
	IsEnabled              string     `bun:"enabled,type:char(1)"`
	HasLoggedOut           string     `bun:"has_logged_out,type:char(1)"`
	RequirePasswordChange  string     `bun:"require_password_change,type:char(1)"`
	LastCurrencyUom        string     `bun:"last_currency_uom,type:varchar(20)"`
	LastLocale             string     `bun:"last_locale,type:varchar(10)"`
	LastTimeZone           string     `bun:"last_time_zone,type:varchar(60)"`
	DisabledDateTime       *time.Time `bun:"disabled_date_time"`
	SuccessiveFailedLogins int        `bun:"successive_failed_logins"`
	ExternalAuthID         string     `bun:"external_auth_id,type:varchar(255)"`
	UserLdapDn             string     `bun:"user_ldap_dn,type:varchar(255)"`
	DisabledBy             string     `bun:"disabled_by,type:varchar(255)"`
	LastUpdatedStamp       time.Time  `bun:"last_updated_stamp"`
	LastUpdatedTxStamp     time.Time  `bun:"last_updated_tx_stamp"`
	CreatedStamp           time.Time  `bun:"created_stamp"`
	CreatedTxStamp         time.Time  `bun:"created_tx_stamp"`
	PartyID                string     `bun:"party_id,type:varchar(20)"`
}
