package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/services"
	"github.com/openndx/openndx-core/internal/utils"
	"gorm.io/gorm"
)

// Handler handles all API requests
type Handler struct {
	policyService *services.PolicyMetadataService
}

// NewHandler creates a new API handler
func NewHandler(db *gorm.DB) *Handler {
	policyService := services.NewPolicyMetadataService(db)
	return &Handler{
		policyService: policyService,
	}
}

// CreatePolicyMetadata handles creating policy metadata
func (h *Handler) CreatePolicyMetadata(w http.ResponseWriter, r *http.Request) {
	var req models.PolicyMetadataCreateRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.SchemaID) == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "schemaId is required and cannot be empty")
		return
	}

	resp, err := h.policyService.CreatePolicyMetadata(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusCreated, resp)
}

// UpdateAllowList handles updating the allow list for a policy
func (h *Handler) UpdateAllowList(w http.ResponseWriter, r *http.Request) {
	var req models.AllowListUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := h.policyService.UpdateAllowList(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// GetPolicyDecision handles getting a policy decision
func (h *Handler) GetPolicyDecision(w http.ResponseWriter, r *http.Request) {
	var req models.PolicyDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.ApplicationID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "applicationId is required")
		return
	}
	if len(req.RequiredFields) == 0 {
		utils.RespondWithError(w, http.StatusBadRequest, "requiredFields is required and cannot be empty")
		return
	}

	resp, err := h.policyService.GetPolicyDecision(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}
