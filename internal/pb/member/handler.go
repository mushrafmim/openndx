package member

import (
	"encoding/json"
	"net/http"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the member HTTP API
type Handler struct {
	service *Service
}

// NewHandler creates a new member Handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateMember handles POST /api/v1/members
func (h *Handler) CreateMember(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission - only admin users can create members
	if !user.HasPermission(auth.PermissionCreateMember) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req CreateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Only admin users should reach this point due to permission check above
	// Admin users can create members for any user if IdpUserID is provided in the request

	member, err := h.service.CreateMember(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), &member.MemberID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, member)
}

// UpdateMember handles PUT /api/v1/members/{memberId}
func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	memberId := r.PathValue("memberId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Get the existing member to check ownership
	existingMember, err := h.service.GetMember(r.Context(), memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Check if user can update this member resource
	// Admin can update any member, regular members can only update their own
	if !user.IsAdmin() && existingMember.IdpUserID != user.IdpUserID {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req UpdateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Pass request context to service for proper context propagation
	member, err := h.service.UpdateMember(r.Context(), memberId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), &existingMember.MemberID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), &member.MemberID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, member)
}

// GetMember handles GET /api/v1/members/{memberId}
func (h *Handler) GetMember(w http.ResponseWriter, r *http.Request) {
	memberId := r.PathValue("memberId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Get the member from database
	// Pass request context to service for proper context propagation
	member, err := h.service.GetMember(r.Context(), memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Check if user can access this member resource
	// Admin can access any member, regular members can only access their own
	if !user.IsAdmin() && member.IdpUserID != user.IdpUserID {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, member)
}

// GetAllMembers handles GET /api/v1/members
func (h *Handler) GetAllMembers(w http.ResponseWriter, r *http.Request) {
	idpUserId := r.URL.Query().Get("idpUserId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission - admin can read all members, regular users need specific permission
	var filteredIdpUserId *string

	if user.HasPermission(auth.PermissionReadAllMembers) {
		// Admin can use provided filters or see all
		filteredIdpUserId = &idpUserId
		// Note: The email query parameter is accepted but not used,
		// since IdpUserID filtering is sufficient for uniqueness
	} else if user.HasPermission(auth.PermissionReadMember) {
		// Regular users can only see their own member record
		// IdpUserID is unique, so no need to also filter by email
		filteredIdpUserId = &user.IdpUserID
	} else {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Pass request context to service for proper context propagation
	// Since IdpUserID is unique, we don't need to pass email parameter
	members, err := h.service.GetAllMembers(r.Context(), filteredIdpUserId, nil)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: members,
		Count: len(members),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}
