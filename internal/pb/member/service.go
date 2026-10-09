package member

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/idp"
	"gorm.io/gorm"
)

// Service handles Member-related operations
type Service struct {
	db  *gorm.DB
	idp idp.IdentityProviderAPI
}

// NewService creates a new Member service
func NewService(db *gorm.DB, idp idp.IdentityProviderAPI) *Service {
	return &Service{db: db, idp: idp}
}

// CreateMember creates a new Member. Normally this also provisions a user in
// the IDP and adds them to the "OpenNDX_Members" group there, so all new
// members are automatically assigned it. If req.IdpUserID is set, the caller
// has already provisioned that user directly in the IDP (e.g. manually via
// ThunderID's console, since idpfactory only supports Asgardeo's admin API
// today) - IDP creation and group assignment are skipped, and the caller is
// responsible for that group/role assignment having already happened.
func (s *Service) CreateMember(ctx context.Context, req *CreateMemberRequest) (*MemberResponse, error) {
	externallyProvisioned := req.IdpUserID != nil
	if externallyProvisioned && *req.IdpUserID == "" {
		return nil, fmt.Errorf("idpUserId must not be empty")
	}

	var idpUserID string
	var groupId *string

	if externallyProvisioned {
		idpUserID = *req.IdpUserID
	} else {
		// Create user in the IDP
		userInstance := &idp.User{
			Email:       req.Email,
			FirstName:   req.Name,
			LastName:    "",
			PhoneNumber: req.PhoneNumber,
		}
		createdUser, err := s.idp.CreateUser(ctx, userInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to create user in IDP: %w", err)
		}
		if createdUser.Email != userInstance.Email {
			deleteErr := (s.idp).DeleteUser(ctx, createdUser.Id)
			if deleteErr != nil {
				return nil, fmt.Errorf("IDP user email mismatch, and failed to rollback user creation in IDP: %w", deleteErr)
			}
			return nil, fmt.Errorf("IDP user email mismatch: expected %s, got %s", userInstance.Email, createdUser.Email)
		}
		slog.Info("Created user in IDP", "userID", createdUser.Id, "email", createdUser.Email)

		// Automatically add user to "OpenNDX_Members" group in the IDP
		// This is a core requirement: all new members must be assigned to the member group
		groupMember := &idp.GroupMember{
			Value:   createdUser.Id,
			Display: createdUser.Email,
		}
		groupId, err = s.idp.AddMemberToGroupByGroupName(ctx, string(UserGroupMember), groupMember)
		if err != nil {
			// Rollback: Delete the user we just created
			deleteErr := s.idp.DeleteUser(ctx, createdUser.Id)
			if deleteErr != nil {
				return nil, fmt.Errorf("failed to add user to group %s: %w (rollback also failed: %v)", UserGroupMember, err, deleteErr)
			}
			return nil, fmt.Errorf("failed to add user to group %s: %w", UserGroupMember, err)
		}
		slog.Info("Added user to group", "userID", createdUser.Id, "groupId", *groupId, "groupName", UserGroupMember)
		idpUserID = createdUser.Id
	}

	// Create Member in the database
	member := Member{
		MemberID:    "mem_" + uuid.New().String(),
		Name:        req.Name,
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		IdpUserID:   idpUserID,
	}
	if dbErr := s.db.Create(&member).Error; dbErr != nil {
		if externallyProvisioned {
			// We didn't create anything in the IDP - nothing to roll back there.
			return nil, fmt.Errorf("failed to create member in database: %w", dbErr)
		}
		// Rollback: Remove user from group and delete user from IDP
		var rollbackErrs []error
		if removeErr := s.idp.RemoveMemberFromGroup(ctx, *groupId, idpUserID); removeErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback group removal: %w", removeErr))
		}
		if deleteErr := s.idp.DeleteUser(ctx, idpUserID); deleteErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback user deletion: %w", deleteErr))
		}
		if len(rollbackErrs) > 0 {
			return nil, fmt.Errorf("failed to create member in database: %w, rollback errors: %v", dbErr, errors.Join(rollbackErrs...))
		}
		return nil, fmt.Errorf("failed to create member in database: %w", dbErr)
	}

	slog.Info("Created member successfully", "memberID", member.MemberID, "email", member.Email)
	return s.buildMemberResponse(&member), nil
}

// UpdateMember updates an existing Member
func (s *Service) UpdateMember(ctx context.Context, memberID string, req *UpdateMemberRequest) (*MemberResponse, error) {
	var member Member
	err := s.db.First(&member, "member_id = ?", memberID).Error
	if err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	// Store original values for rollback if needed
	originalName := member.Name
	originalPhoneNumber := member.PhoneNumber

	// Check if we need to update the IDP user
	needsIdpUpdate := false

	// Update fields if provided
	if req.Name != nil {
		member.Name = *req.Name
		needsIdpUpdate = true
	}
	if req.PhoneNumber != nil {
		member.PhoneNumber = *req.PhoneNumber
		needsIdpUpdate = true
	}

	// Update user in IDP if necessary
	if needsIdpUpdate {
		userInstance := &idp.User{
			Email:       member.Email,
			FirstName:   member.Name,
			LastName:    "",
			PhoneNumber: member.PhoneNumber,
		}

		_, err := s.idp.UpdateUser(ctx, member.IdpUserID, userInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to update user in IDP: %w", err)
		}

		slog.Info("Updated user in IDP", "userID", member.IdpUserID)
	}

	// Update member in database
	if err := s.db.Save(&member).Error; err != nil {
		// Rollback IDP user update if DB operation fails
		if needsIdpUpdate {
			rollbackUser := &idp.User{
				Email:       member.Email,
				FirstName:   originalName,
				LastName:    "",
				PhoneNumber: originalPhoneNumber,
			}
			_, rollbackErr := s.idp.UpdateUser(ctx, member.IdpUserID, rollbackUser)
			if rollbackErr != nil {
				return nil, fmt.Errorf("failed to update member in database and failed to rollback IDP update: %w", errors.Join(err, fmt.Errorf("failed to rollback IDP update: %w", rollbackErr)))
			}
			slog.Warn("Rolled back IDP user update due to database failure", "userID", member.IdpUserID)
		}
		return nil, fmt.Errorf("failed to update member in database: %w", err)
	}

	slog.Info("Updated member successfully", "memberID", memberID)
	return s.buildMemberResponse(&member), nil
}

// GetMember retrieves a Member by ID
func (s *Service) GetMember(ctx context.Context, memberID string) (*MemberResponse, error) {
	var member Member
	err := s.db.Where("member_id = ?", memberID).First(&member).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch member: %w", err)
	}

	return s.buildMemberResponse(&member), nil
}

// GetAllMembers retrieves all members, optionally filtered by idpUserId or email
func (s *Service) GetAllMembers(ctx context.Context, idpUserId *string, email *string) ([]MemberResponse, error) {
	// Handle filtered query
	if (idpUserId != nil && *idpUserId != "") || (email != nil && *email != "") {
		return s.getFilteredMembers(ctx, idpUserId, email)
	}

	// Handle all members query
	return s.getAllMembers(ctx)
}

// getFilteredMembers retrieves members filtered by idpUserId or email
func (s *Service) getFilteredMembers(ctx context.Context, idpUserId *string, email *string) ([]MemberResponse, error) {
	var member Member
	query := s.db
	if idpUserId != nil && *idpUserId != "" {
		query = query.Where("idp_user_id = ?", *idpUserId)
	}
	if email != nil && *email != "" {
		query = query.Where("email = ?", *email)
	}
	err := query.First(&member).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch member: %w", err)
	}

	return []MemberResponse{*s.buildMemberResponse(&member)}, nil
}

// getAllMembers retrieves all members
func (s *Service) getAllMembers(ctx context.Context) ([]MemberResponse, error) {
	var members []Member
	err := s.db.Find(&members).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch members: %w", err)
	}

	response := make([]MemberResponse, len(members))
	for i, member := range members {
		response[i] = *s.buildMemberResponse(&member)
	}

	return response, nil
}

// ResolveMemberID returns the member ID of the authenticated user, caching the
// result (or the lookup error) on the user so repeated calls within the same
// request avoid extra database lookups.
func (s *Service) ResolveMemberID(ctx context.Context, user *auth.AuthenticatedUser) (string, error) {
	// Check if we already have cached the member ID
	if memberID, cached := user.GetCachedMemberID(); cached {
		// Return cached error if the previous lookup failed
		if err := user.GetCachedMemberIDError(); err != nil {
			return "", err
		}
		return memberID, nil
	}

	// Not cached, perform the database lookup
	members, err := s.GetAllMembers(ctx, &user.IdpUserID, nil)
	if err != nil {
		user.SetCachedMemberID("", err)
		return "", err
	}

	if len(members) == 0 {
		err = fmt.Errorf("user member record not found")
		user.SetCachedMemberID("", err)
		return "", err
	}

	// Cache the successful result
	memberID := members[0].MemberID
	user.SetCachedMemberID(memberID, nil)
	return memberID, nil
}

// buildMemberResponse converts a Member model to MemberResponse
func (s *Service) buildMemberResponse(member *Member) *MemberResponse {
	return &MemberResponse{
		MemberID:    member.MemberID,
		IdpUserID:   member.IdpUserID,
		Name:        member.Name,
		Email:       member.Email,
		PhoneNumber: member.PhoneNumber,
		CreatedAt:   member.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   member.UpdatedAt.Format(time.RFC3339),
	}
}
