package member

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/auth"
	"gorm.io/gorm"
)

// Service handles Member-related operations
type Service struct {
	db *gorm.DB
}

// NewService creates a new Member service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateMember creates a new Member organization linked to an existing
// ThunderID organization unit. The OU and its users are managed in ThunderID;
// nothing is provisioned in the IDP here.
func (s *Service) CreateMember(ctx context.Context, req *CreateMemberRequest) (*MemberResponse, error) {
	if req.OuID == "" {
		return nil, fmt.Errorf("ouId must not be empty")
	}

	member := Member{
		MemberID:    "mem_" + uuid.New().String(),
		Name:        req.Name,
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		OuID:        req.OuID,
	}
	if err := s.db.Create(&member).Error; err != nil {
		return nil, fmt.Errorf("failed to create member in database: %w", err)
	}

	slog.Info("Created member successfully", "memberID", member.MemberID, "ouID", member.OuID)
	return s.buildMemberResponse(&member), nil
}

// UpdateMember updates an existing Member. The OU link cannot be changed.
func (s *Service) UpdateMember(ctx context.Context, memberID string, req *UpdateMemberRequest) (*MemberResponse, error) {
	var member Member
	err := s.db.First(&member, "member_id = ?", memberID).Error
	if err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	if req.Name != nil {
		member.Name = *req.Name
	}
	if req.PhoneNumber != nil {
		member.PhoneNumber = *req.PhoneNumber
	}

	if err := s.db.Save(&member).Error; err != nil {
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

// GetAllMembers retrieves all members, optionally filtered by ouId
func (s *Service) GetAllMembers(ctx context.Context, ouID *string) ([]MemberResponse, error) {
	// Handle filtered query
	if ouID != nil && *ouID != "" {
		return s.getMembersByOuID(ctx, *ouID)
	}

	// Handle all members query
	return s.getAllMembers(ctx)
}

// getMembersByOuID retrieves the member linked to the given organization unit
func (s *Service) getMembersByOuID(ctx context.Context, ouID string) ([]MemberResponse, error) {
	var member Member
	err := s.db.Where("ou_id = ?", ouID).First(&member).Error
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

// ResolveMemberID returns the member ID of the authenticated user's
// organization unit, caching the result (or the lookup error) on the user so
// repeated calls within the same request avoid extra database lookups.
func (s *Service) ResolveMemberID(ctx context.Context, user *auth.AuthenticatedUser) (string, error) {
	// Check if we already have cached the member ID
	if memberID, cached := user.GetCachedMemberID(); cached {
		// Return cached error if the previous lookup failed
		if err := user.GetCachedMemberIDError(); err != nil {
			return "", err
		}
		return memberID, nil
	}

	if user.OuID == "" {
		err := fmt.Errorf("user has no organization unit claim")
		user.SetCachedMemberID("", err)
		return "", err
	}

	// Not cached, perform the database lookup
	members, err := s.GetAllMembers(ctx, &user.OuID)
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
		Name:        member.Name,
		Email:       member.Email,
		PhoneNumber: member.PhoneNumber,
		OuID:        member.OuID,
		CreatedAt:   member.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   member.UpdatedAt.Format(time.RFC3339),
	}
}
