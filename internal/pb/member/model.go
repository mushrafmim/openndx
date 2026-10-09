// Package member is the Portal Backend's member bounded context: member
// records, their IdP accounts, and the member API.
package member

import "github.com/openndx/openndx-core/internal/pb/kernel"

// UserGroup represents different user groups in the system
type UserGroup string

const (
	UserGroupAdmin  UserGroup = "OpenNDX_Admin"
	UserGroupMember UserGroup = "OpenNDX_Members"
)

// Member represents the normalized entity table
type Member struct {
	MemberID    string `gorm:"primarykey;column:member_id" json:"memberId"`
	Name        string `gorm:"column:name;not null" json:"name"`
	Email       string `gorm:"column:email;not null;unique" json:"email"`
	PhoneNumber string `gorm:"column:phone_number;not null" json:"phoneNumber"`
	IdpUserID   string `gorm:"column:idp_user_id;not null;unique" json:"idpUserId"`
	kernel.BaseModel
}

// TableName sets the table name for GORM
func (Member) TableName() string {
	return "members"
}
