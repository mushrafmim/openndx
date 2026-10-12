package auth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    FlexibleStringSlice
		wantErr bool
	}{
		{
			name:    "single string",
			json:    `"test"`,
			want:    FlexibleStringSlice{"test"},
			wantErr: false,
		},
		{
			name:    "string array",
			json:    `["test1", "test2"]`,
			want:    FlexibleStringSlice{"test1", "test2"},
			wantErr: false,
		},
		{
			name:    "empty string",
			json:    `""`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty array",
			json:    `[]`,
			want:    FlexibleStringSlice{},
			wantErr: false,
		},
		{
			name:    "invalid type",
			json:    `123`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid string in array",
			json:    `["test", ""]`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleStringSlice
			err := json.Unmarshal([]byte(tt.json), &f)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, f)
			}
		})
	}
}

func TestFlexibleStringSlice_ToStringSlice(t *testing.T) {
	f := FlexibleStringSlice{"a", "b"}
	assert.Equal(t, []string{"a", "b"}, f.ToStringSlice())
}

func TestUserClaims_OuID(t *testing.T) {
	payload := `{"sub":"user-1","ouId":"ou-123","roles":["OpenNDX_Member"]}`

	var claims UserClaims
	assert.NoError(t, json.Unmarshal([]byte(payload), &claims))
	assert.Equal(t, "ou-123", claims.OuID)

	user, err := NewAuthenticatedUser(&claims)
	assert.NoError(t, err)
	assert.Equal(t, "ou-123", user.OuID)
}
