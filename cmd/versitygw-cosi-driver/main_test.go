package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvOrDefault_ReturnsEnvValue(t *testing.T) {
	t.Setenv("TEST_ENV_OR_DEFAULT_SET", "from-env")

	got := envOrDefault("TEST_ENV_OR_DEFAULT_SET", "fallback")
	require.Equal(t, "from-env", got)
}

func TestEnvOrDefault_ReturnsDefault(t *testing.T) {
	got := envOrDefault("TEST_ENV_OR_DEFAULT_UNSET_KEY", "fallback")
	require.Equal(t, "fallback", got)
}
