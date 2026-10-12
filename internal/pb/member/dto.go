package member

type CreateMemberRequest struct {
	Name        string `json:"name" validate:"required"`
	Email       string `json:"email" validate:"required,email"`
	PhoneNumber string `json:"phoneNumber" validate:"required"`
	// OuID is the ThunderID organization unit this member maps to. The OU and
	// its users are managed in ThunderID; PB only stores the link.
	OuID string `json:"ouId" validate:"required"`
}

type UpdateMemberRequest struct {
	Name        *string `json:"name,omitempty"`
	PhoneNumber *string `json:"phoneNumber,omitempty"`
}

type MemberResponse struct {
	MemberID    string `json:"memberId"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	OuID        string `json:"ouId"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
