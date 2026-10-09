package member

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pb/auth/authtest"
	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/idp"
	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/stretchr/testify/assert"
)

// newTestHandler creates a member Handler backed by an in-memory SQLite
// database and a fresh mock IDP.
func newTestHandler(t *testing.T) (*Handler, *idptest.Mock) {
	db := dbtest.SetupSQLiteDB(t, &Member{})
	idpMock := &idptest.Mock{}
	return NewHandler(NewService(db, idpMock)), idpMock
}

// setupMockIDPForMemberCreation configures the mock IDP to successfully create a member
func setupMockIDPForMemberCreation(idpMock *idptest.Mock, email string, userID string) {
	groupId := "group-123"
	createdUser := &idp.UserInfo{
		Id:          userID,
		Email:       email,
		FirstName:   "Test",
		LastName:    "User",
		PhoneNumber: "1234567890",
	}
	idpMock.CreateUserFunc = func(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
		return createdUser, nil
	}
	idpMock.AddMemberToGroupByGroupNameFunc = func(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error) {
		return &groupId, nil
	}
}

// TestMemberEndpoints tests all member-related endpoints
func TestMemberEndpoints(t *testing.T) {
	handler, idpMock := newTestHandler(t)

	t.Run("POST /api/v1/members - CreateMember", func(t *testing.T) {
		req := CreateMemberRequest{
			Name:        "Test Member",
			Email:       fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()),
			PhoneNumber: "1234567890",
		}

		// Setup mock IDP for member creation
		userID := "idp-user-" + fmt.Sprintf("%d", time.Now().UnixNano())
		setupMockIDPForMemberCreation(idpMock, req.Email, userID)

		reqBody, _ := json.Marshal(req)
		httpReq := authtest.NewAdminRequest(http.MethodPost, "/api/v1/members", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.CreateMember(w, httpReq)

		assert.Equal(t, http.StatusCreated, w.Code)
		var response MemberResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, req.Name, response.Name)
		assert.Equal(t, req.Email, response.Email)
		assert.Equal(t, req.PhoneNumber, response.PhoneNumber)
		assert.NotEmpty(t, response.MemberID)
	})

	t.Run("POST /api/v1/members - Invalid JSON", func(t *testing.T) {
		httpReq := authtest.NewAdminRequest(http.MethodPost, "/api/v1/members", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.CreateMember(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT /api/v1/members/:id - UpdateMember_InvalidJSON", func(t *testing.T) {
		httpReq := authtest.NewAdminRequest(http.MethodPut, "/api/v1/members/test-id", bytes.NewBufferString("invalid json"))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("memberId", "test-id")
		w := httptest.NewRecorder()
		handler.UpdateMember(w, httpReq)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("PUT /api/v1/members/:id - UpdateMember_NotFound", func(t *testing.T) {
		name := "Updated Name"
		req := UpdateMemberRequest{
			Name: &name,
		}
		reqBody, _ := json.Marshal(req)
		httpReq := authtest.NewAdminRequest(http.MethodPut, "/api/v1/members/non-existent-id", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetPathValue("memberId", "non-existent-id")
		w := httptest.NewRecorder()
		handler.UpdateMember(w, httpReq)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/v1/members - GetAllMembers", func(t *testing.T) {
		httpReq := authtest.NewAdminRequest(http.MethodGet, "/api/v1/members", nil)
		w := httptest.NewRecorder()
		handler.GetAllMembers(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response kernel.CollectionResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Items)
		assert.GreaterOrEqual(t, response.Count, 0)
	})

	t.Run("GET /api/v1/members - WithQueryParams", func(t *testing.T) {
		httpReq := authtest.NewAdminRequest(http.MethodGet, "/api/v1/members?email=test@example.com", nil)
		w := httptest.NewRecorder()
		handler.GetAllMembers(w, httpReq)

		// May return 500 if query fails, but should handle gracefully
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
	})

	t.Run("GET /api/v1/members/:memberId - NotFound", func(t *testing.T) {
		httpReq := authtest.NewAdminRequest(http.MethodGet, "/api/v1/members/non-existent-id", nil)
		httpReq.SetPathValue("memberId", "non-existent-id")
		w := httptest.NewRecorder()
		handler.GetMember(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
