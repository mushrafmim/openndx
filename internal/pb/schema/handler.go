package schema

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openndx/openndx-core/internal/pb/auth"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/utils"
)

// memberResolver resolves the member ID of an authenticated user
type memberResolver interface {
	ResolveMemberID(ctx context.Context, user *auth.AuthenticatedUser) (string, error)
}

// Handler handles the schema, schema submission and schema policy metadata HTTP API
type Handler struct {
	service *Service
	members memberResolver
}

// NewHandler creates a new schema Handler. members resolves the member ID of
// the authenticated user (see member.Service).
func NewHandler(service *Service, members memberResolver) *Handler {
	return &Handler{service: service, members: members}
}

// getUserMemberID gets the member ID for the authenticated user with caching
// This avoids repeated database calls for the same user within the same request context
func (h *Handler) getUserMemberID(r *http.Request, user *auth.AuthenticatedUser) (string, error) {
	return h.members.ResolveMemberID(r.Context(), user)
}

// Schema submission handlers

// GetAllSchemaSubmissions handles GET /api/v1/schema-submissions
func (h *Handler) GetAllSchemaSubmissions(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")
	statusFilter := r.URL.Query()["status"]

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	var filteredMemberId *string
	if user.HasPermission(auth.PermissionReadAllSchemaSubmissions) {
		// Admin/System can use provided filters or see all
		filteredMemberId = &memberId
	} else if user.HasPermission(auth.PermissionReadSchemaSubmission) {
		// Regular users can only see their own submissions
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

	submissions, err := h.service.GetSchemaSubmissions(filteredMemberId, &statusFilter)
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

// GetSchemaSubmission handles GET /api/v1/schema-submissions/{submissionId}
func (h *Handler) GetSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionReadSchemaSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	submission, err := h.service.GetSchemaSubmission(submissionId)
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

// CreateSchemaSubmission handles POST /api/v1/schema-submissions
func (h *Handler) CreateSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionCreateSchemaSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req CreateSchemaSubmissionRequest
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

	submission, err := h.service.CreateSchemaSubmission(&req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, submission)
}

// UpdateSchemaSubmission handles PUT /api/v1/schema-submissions/{submissionId}
func (h *Handler) UpdateSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionUpdateSchemaSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing submission to check ownership
	existingSubmission, err := h.service.GetSchemaSubmission(submissionId)
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
		if existingSubmission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req UpdateSchemaSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	submission, err := h.service.UpdateSchemaSubmission(submissionId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), &existingSubmission.SubmissionID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// Schema handlers

// GetAllSchemas handles GET /api/v1/schemas
func (h *Handler) GetAllSchemas(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionReadSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// For non-admin users, filter results to only their own schemas
	var filteredMemberId *string
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberId, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		filteredMemberId = &userMemberId
	} else {
		// Admin can specify memberId or see all
		filteredMemberId = &memberId
	}

	schemas, err := h.service.GetSchemas(filteredMemberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: schemas,
		Count: len(schemas),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetSchema handles GET /api/v1/schemas/{schemaId}
func (h *Handler) GetSchema(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionReadSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	schema, err := h.service.GetSchema(schemaId)
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

		// Check if schema belongs to the user
		if schema.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, schema)
}

// CreateSchema handles POST /api/v1/schemas
func (h *Handler) CreateSchema(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionCreateSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req CreateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create schemas for themselves
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

	schema, err := h.service.CreateSchema(&req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schema.SchemaID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, schema)
}

// UpdateSchema handles PUT /api/v1/schemas/{schemaId}
func (h *Handler) UpdateSchema(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")

	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(auth.PermissionUpdateSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing schema to check ownership
	existingSchema, err := h.service.GetSchema(schemaId)
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

		// Check if schema belongs to the user
		if existingSchema.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req UpdateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	schema, err := h.service.UpdateSchema(schemaId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &existingSchema.SchemaID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schema.SchemaID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, schema)
}

// Schema policy metadata handlers

// authorizeSchemaAccess checks that the authenticated user has the permission and,
// for non-admin users, owns the schema. It writes the error response and returns
// false when access is denied.
func (h *Handler) authorizeSchemaAccess(w http.ResponseWriter, r *http.Request, schemaId string, permission auth.Permission) bool {
	// Get authenticated user
	user, err := auth.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return false
	}

	// Check permission
	if !user.HasPermission(permission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return false
	}

	schema, err := h.service.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return false
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return false
		}

		// Check if schema belongs to the user
		if schema.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return false
		}
	}

	return true
}

// ListSchemaPolicyMetadata handles GET /api/v1/schemas/{schemaId}/policy-metadata
func (h *Handler) ListSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	if !h.authorizeSchemaAccess(w, r, schemaId, auth.PermissionReadSchema) {
		return
	}

	list, err := h.service.ListPolicyMetadata(schemaId)
	if err != nil {
		respondWithPolicyMetadataError(w, err)
		return
	}

	response := kernel.CollectionResponse{
		Items: list.Records,
		Count: len(list.Records),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// PatchSchemaPolicyMetadata handles PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}
func (h *Handler) PatchSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	if !h.authorizeSchemaAccess(w, r, schemaId, auth.PermissionUpdateSchema) {
		return
	}

	var req policy.PolicyMetadataPatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IsEmpty() {
		utils.RespondWithError(w, http.StatusBadRequest, "at least one editable field is required")
		return
	}

	record, err := h.service.PatchPolicyMetadata(schemaId, id, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, record)
}

// DeleteSchemaPolicyMetadata handles DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}
func (h *Handler) DeleteSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	if !h.authorizeSchemaAccess(w, r, schemaId, auth.PermissionUpdateSchema) {
		return
	}

	if err := h.service.DeletePolicyMetadata(schemaId, id); err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusSuccess))

	w.WriteHeader(http.StatusNoContent)
}

// RevokeSchemaPolicyAllowListEntry handles DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}/allowlist/{applicationId}
func (h *Handler) RevokeSchemaPolicyAllowListEntry(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	applicationId := r.PathValue("applicationId")
	if !h.authorizeSchemaAccess(w, r, schemaId, auth.PermissionUpdateSchema) {
		return
	}

	if err := h.service.RevokeAllowListEntry(schemaId, id, applicationId); err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusSuccess))

	w.WriteHeader(http.StatusNoContent)
}

// respondWithPolicyMetadataError maps policy metadata errors onto a response.
// Client errors reported by the PDP are passed through, while PDP failures and
// unreachable PDPs are reported as a bad gateway.
func respondWithPolicyMetadataError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrPolicyMetadataNotFound) {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	var pdpErr *policy.Error
	if errors.As(err, &pdpErr) {
		switch pdpErr.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
			utils.RespondWithError(w, pdpErr.StatusCode, pdpErr.Message)
			return
		}
	}

	slog.Error("Policy metadata request to PDP failed", "error", err)
	utils.RespondWithError(w, http.StatusBadGateway, "Policy decision point request failed")
}
