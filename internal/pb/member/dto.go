package member

type CreateMemberRequest struct {
	Name        string `json:"name" validate:"required"`
	Email       string `json:"email" validate:"required,email"`
	PhoneNumber string `json:"phoneNumber" validate:"required"`
	// IdpUserID lets a caller register a member whose IDP account was already
	// provisioned directly (e.g. manually via ThunderID's console, since
	// idpfactory only implements Asgardeo's admin API today - see
	// internal/pb/idp/idpfactory). When set, creation skips creating the user
	// and assigning them to the member group in the IDP entirely, and uses
	// this value as-is - the caller is responsible for that group/role
	// assignment having already happened.
	IdpUserID *string `json:"idpUserId,omitempty"`
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
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	IdpUserID   string `json:"idpUserId"`
}
