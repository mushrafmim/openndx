package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClientFromEnv(t *testing.T) {
	t.Run("missing PDP_SERVICE_URL", func(t *testing.T) {
		t.Setenv("PDP_SERVICE_URL", "")

		client, err := NewClientFromEnv()
		assert.EqualError(t, err, "PDP_SERVICE_URL environment variable not set")
		assert.Nil(t, client)
	})

	t.Run("URL without http(s) scheme", func(t *testing.T) {
		t.Setenv("PDP_SERVICE_URL", "pdp:8080")

		client, err := NewClientFromEnv()
		assert.EqualError(t, err, "PDP_SERVICE_URL must start with http:// or https://")
		assert.Nil(t, client)
	})

	t.Run("valid URL", func(t *testing.T) {
		t.Setenv("PDP_SERVICE_URL", "http://pdp:8080/")

		client, err := NewClientFromEnv()
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "http://pdp:8080", client.baseURL)
	})
}
