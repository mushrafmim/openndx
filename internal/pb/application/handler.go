package application

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/utils"
)

// memberResolver resolves the member ID of an authenticated user
type memberResolver interface {
	ResolveMemberID(ctx context.Context, user *auth.AuthenticatedUser) (string, error)
}

// Handler handles the application and application submission HTTP API
type Handler struct {
	members memberResolver
	service *Service
}

// getUserMemberID gets the member ID for the authenticated user with caching
// This avoids repeated database calls for the same user within the same request context
func (h *Handler) getUserMemberID(r *http.Request, user *auth.AuthenticatedUser) (string, error) {
	return h.members.ResolveMemberID(r.Context(), user)
}

// NewHandler creates a new application Handler. members resolves the member ID
// of the authenticated user (see member.Service).
func NewHandler(service *Service, members memberResolver) *Handler {
	return &Handler{service: service, members: members}
}

// Application submission handlers

// GetAllApplicationSubmissions handles GET /api/v1/application-submissions
func (h *Handler) GetAllApplicationSubmissions(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")
	statusFilter := r.URL.Query()["status"]

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionReadApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var finalMemberId *string = &memberId

	// For non-admin users, force filtering to their own submissions only
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Force the memberId to the authenticated user's member ID
		finalMemberId = &userMemberID
	}

	submissions, err := h.service.GetApplicationSubmissions(r.Context(), finalMemberId, &statusFilter)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: submissions,
		Count: len(submissions),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetApplicationSubmission handles GET /api/v1/application-submissions/{submissionId}
func (h *Handler) GetApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionReadApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	submission, err := h.service.GetApplicationSubmission(r.Context(), submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if submission belongs to the user
		if submission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// CreateApplicationSubmission handles POST /api/v1/application-submissions
func (h *Handler) CreateApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionCreateApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req CreateApplicationSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create submissions for themselves
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// If MemberID is provided, validate ownership
		if req.MemberID != "" {
			// Check if the provided MemberID belongs to the authenticated user
			if req.MemberID != userMemberID {
				utils.RespondWithError(w, http.StatusForbidden, "Access denied: cannot create submission for another user")
				return
			}
		} else {
			// If no MemberID provided, set it to the authenticated user's member ID
			req.MemberID = userMemberID
		}
	}

	submission, err := h.service.CreateApplicationSubmission(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, submission)
}

// UpdateApplicationSubmission handles PUT /api/v1/application-submissions/{submissionId}
func (h *Handler) UpdateApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionUpdateApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing submission to check ownership
	existingSubmission, err := h.service.GetApplicationSubmission(r.Context(), submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, "Application submission not found")
		return
	}

	// For non-admin users, check ownership before updating
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if submission belongs to the user
		if existingSubmission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	var req UpdateApplicationSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	submission, err := h.service.UpdateApplicationSubmission(r.Context(), submissionId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), &existingSubmission.SubmissionID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// Application handlers

// GetAllApplications handles GET /api/v1/applications
func (h *Handler) GetAllApplications(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	var filteredMemberId *string
	if user.HasPermission(auth.PermissionReadAllApplications) {
		// Admin/System can use provided filters or see all
		filteredMemberId = &memberId
	} else if user.HasPermission(auth.PermissionReadApplication) {
		// Regular users can only see their own applications
		// Get member ID for the authenticated user (cached)
		userMemberId, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		filteredMemberId = &userMemberId
	} else {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	applications, err := h.service.GetApplications(r.Context(), filteredMemberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: applications,
		Count: len(applications),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetApplication handles GET /api/v1/applications/{applicationId}
func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionReadApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	application, err := h.service.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if application belongs to the user
		if application.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, application)
}

// GetApplicationIdByClientId handles GET /internal/api/v1/applications
func (h *Handler) GetApplicationIdByClientId(w http.ResponseWriter, r *http.Request) {
	idpClientId := r.URL.Query().Get("idpClientId")
	if idpClientId == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "idpClientId query parameter is required")
		return
	}

	applicationId, err := h.service.GetApplicationIdByIdpClientId(r.Context(), idpClientId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}
	utils.RespondWithSuccess(w, http.StatusOK, applicationId)
}

// CreateApplication handles POST /api/v1/applications
func (h *Handler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionCreateApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req CreateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create applications for themselves
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Set the member ID to the authenticated user's member ID
		req.MemberID = userMemberID
	}

	application, err := h.service.CreateApplication(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &application.ApplicationID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, application)
}

// UpdateApplication handles PUT /api/v1/applications/{applicationId}
func (h *Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionUpdateApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing application to check ownership
	existingApplication, err := h.service.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if application belongs to the user
		if existingApplication.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req UpdateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	application, err := h.service.UpdateApplication(r.Context(), applicationId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &existingApplication.ApplicationID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &application.ApplicationID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, application)
}

// UpdateApplicationPolicy handles PUT /api/v1/applications/{applicationId}/policy
func (h *Handler) UpdateApplicationPolicy(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission - policy updates share the application:update permission
	if !user.HasPermission(auth.PermissionUpdateApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing application to check ownership
	existingApplication, err := h.service.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if application belongs to the user
		if existingApplication.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req UpdateApplicationPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	application, err := h.service.UpdateApplicationPolicy(r.Context(), applicationId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &existingApplication.ApplicationID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &application.ApplicationID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, application)
}
