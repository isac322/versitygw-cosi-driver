package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Parallel()

	info := Get()

	// GoVersion is always available from runtime/debug
	require.NotEmpty(t, info.GoVersion)
	require.NotEqual(t, "(unknown)", info.GoVersion)
}
