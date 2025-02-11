package apikeygen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/internal/util"
)

func TestApiKeyGen(t *testing.T) {
	result, err := New(12, "Cool key")
	require.NoError(t, err)

	assert.NotEmpty(t, result.ClientSecret)
	assert.NotEmpty(t, result.HashedKey)

	keyInfo, err := Decode(result.ClientSecret)
	require.NoError(t, err)

	keyHashed, err := util.EncodePassword(keyInfo.Key, keyInfo.Name)
	require.NoError(t, err)
	assert.Equal(t, result.HashedKey, keyHashed)
}
