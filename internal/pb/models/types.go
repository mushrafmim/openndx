package models

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/openndx/openndx-core/internal/pb/policy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SelectedFieldRecords represents an array of policy.SelectedFieldRecord with custom scanning
type SelectedFieldRecords []policy.SelectedFieldRecord

// Scan implements the sql.Scanner interface for SelectedFieldRecords
func (sfr *SelectedFieldRecords) Scan(value interface{}) error {
	if value == nil {
		*sfr = SelectedFieldRecords{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into SelectedFieldRecords", value)
	}

	return json.Unmarshal(bytes, sfr)
}

// Value implements the driver.Valuer interface for SelectedFieldRecords
func (sfr *SelectedFieldRecords) Value() (driver.Value, error) {
	return json.Marshal(*sfr)
}

// GormDataType gorm common data type
func (SelectedFieldRecords) GormDataType() string {
	return "jsonb"
}

// GormValue implements the GormValuerInterface
func (sfr SelectedFieldRecords) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	data, err := json.Marshal(sfr)
	if err != nil {
		// Panic on marshaling error to prevent silent data loss
		// JSON marshaling of SelectedFieldRecords should never fail under normal circumstances
		panic(fmt.Sprintf("Failed to marshal SelectedFieldRecords to JSON: %v", err))
	}

	// Use dialect-appropriate syntax
	// PostgreSQL uses ::jsonb cast, SQLite uses JSON() function or plain ?
	dialector := db.Dialector.Name()
	var sql string
	if dialector == "postgres" {
		sql = "?::jsonb"
	} else {
		// SQLite and other databases - use plain parameter
		// GORM will handle JSON encoding automatically
		sql = "?"
	}

	return clause.Expr{
		SQL:  sql,
		Vars: []interface{}{string(data)},
	}
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
