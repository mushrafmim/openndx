package auth

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// UserClaims represents the JWT claims for a user
type UserClaims struct {
	Email       string              `json:"email"`
	FirstName   string              `json:"given_name"`
	LastName    string              `json:"family_name"`
	PhoneNumber string              `json:"phone_number"`
	Roles       FlexibleStringSlice `json:"roles"`
	Groups      FlexibleStringSlice `json:"groups"`
	IdpUserID   string              `json:"sub"` // Subject is typically the user ID from IdP
	// Standard JWT claims - using int64 for Unix timestamps
	Issuer    string              `json:"iss"`
	Audience  FlexibleStringSlice `json:"aud"`
	ExpiresAt int64               `json:"exp"`
	IssuedAt  int64               `json:"iat"`
	NotBefore int64               `json:"nbf"`
}

// GetExpirationTime implements jwt.Claims interface
func (c *UserClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.ExpiresAt, 0)), nil
}

// GetIssuedAt implements jwt.Claims interface
func (c *UserClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.IssuedAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.IssuedAt, 0)), nil
}

// GetNotBefore implements jwt.Claims interface
func (c *UserClaims) GetNotBefore() (*jwt.NumericDate, error) {
	if c.NotBefore == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.NotBefore, 0)), nil
}

// GetIssuer implements jwt.Claims interface
func (c *UserClaims) GetIssuer() (string, error) {
	return c.Issuer, nil
}

// GetSubject implements jwt.Claims interface
func (c *UserClaims) GetSubject() (string, error) {
	return c.IdpUserID, nil
}

// GetAudience implements jwt.Claims interface
func (c *UserClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings(c.Audience.ToStringSlice()), nil
}

// FlexibleStringSlice can unmarshal both single string and string array from JSON
type FlexibleStringSlice []string

// UnmarshalJSON implements custom unmarshaling to handle both string and []string
func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string array first
	var strArray []string
	arrayErr := json.Unmarshal(data, &strArray)
	if arrayErr == nil {
		// Validate each string in the array
		if err := validateStringSlice(strArray); err != nil {
			return fmt.Errorf("invalid string array: %v", err)
		}
		*f = FlexibleStringSlice(strArray)
		return nil
	}

	// If that fails, try to unmarshal as single string
	var str string
	stringErr := json.Unmarshal(data, &str)
	if stringErr == nil {
		// Validate the single string
		if err := validateString(str); err != nil {
			return fmt.Errorf("invalid string: %v", err)
		}
		*f = FlexibleStringSlice([]string{str})
		return nil
	}

	// If both fail, return a detailed error with both attempts
	return fmt.Errorf("failed to unmarshal FlexibleStringSlice: cannot parse as []string (%v) or string (%v), data: %s",
		arrayErr, stringErr, string(data))
}

// ToStringSlice converts to regular string slice
func (f *FlexibleStringSlice) ToStringSlice() []string {
	return []string(*f)
}

// validateString validates a single string for security concerns
func validateString(s string) error {
	// Check for empty strings (often used in bypass attempts)
	if len(s) == 0 {
		return fmt.Errorf("empty string not allowed")
	}

	// Check for excessively long strings (potential DoS or buffer overflow attempts)
	const maxStringLength = 1024
	if len(s) > maxStringLength {
		return fmt.Errorf("string too long (max %d characters)", maxStringLength)
	}

	// Check for null bytes (potential injection attempts)
	for i, b := range []byte(s) {
		if b == 0 {
			return fmt.Errorf("null byte found at position %d", i)
		}
	}

	return nil
}

// validateStringSlice validates all strings in a slice
func validateStringSlice(slice []string) error {
	// Check for excessively large arrays (potential DoS)
	const maxArrayLength = 100
	if len(slice) > maxArrayLength {
		return fmt.Errorf("array too large (max %d elements)", maxArrayLength)
	}

	// Validate each individual string
	for i, s := range slice {
		if err := validateString(s); err != nil {
			return fmt.Errorf("invalid string at index %d: %v", i, err)
		}
	}

	return nil
}
