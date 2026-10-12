// Package member is the Portal Backend's member bounded context: member
// organizations, their link to the IdP, and the member API.
package member

import "github.com/openndx/openndx-core/internal/pb/kernel"

// Member represents a member organization. Its users live in the IdP, under
// the ThunderID organization unit identified by OuID.
type Member struct {
	MemberID    string `gorm:"primarykey;column:member_id" json:"memberId"`
	Name        string `gorm:"column:name;not null" json:"name"`
	Email       string `gorm:"column:email;not null;unique" json:"email"`
	PhoneNumber string `gorm:"column:phone_number;not null" json:"phoneNumber"`
	OuID        string `gorm:"column:ou_id;not null;unique" json:"ouId"`
	kernel.BaseModel
}

// TableName sets the table name for GORM
func (Member) TableName() string {
	return "members"
}
