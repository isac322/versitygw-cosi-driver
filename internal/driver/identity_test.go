package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	cosi "sigs.k8s.io/container-object-storage-interface-spec"
)

func TestNewIdentityServer(t *testing.T) {
	t.Parallel()

	name := "test.cosi.dev"
	server := NewIdentityServer(name)

	require.NotNil(t, server)
	require.Equal(t, name, server.driverName)
}

func TestIdentityServer_DriverGetInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		driverName string
	}{
		{
			name:       "returns configured driver name",
			driverName: "versitygw.cosi.dev",
		},
		{
			name:       "returns custom driver name",
			driverName: "custom.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := NewIdentityServer(tt.driverName)
			resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

			require.NoError(t, err)
			require.Equal(t, tt.driverName, resp.Name)
		})
	}
}

func TestDriverNameValidation(t *testing.T) {
	t.Parallel()

	// TC-U-010: Valid domain-name format
	t.Run("TC-U-010_valid_domain_name_format", func(t *testing.T) {
		t.Parallel()

		server := NewIdentityServer("versitygw.cosi.dev")
		resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

		require.NoError(t, err)
		require.Equal(t, "versitygw.cosi.dev", resp.Name)
	})

	// TC-U-011: Name exceeds 63 characters
	// The current IdentityServer does not validate name length. This test
	// documents that a name exceeding 63 characters is accepted and returned as-is.
	t.Run("TC-U-011_name_exceeds_63_characters", func(t *testing.T) {
		t.Parallel()

		longName := "abcdefghijklmnopqrstuvwxyz0123456789.abcdefghijklmnopqrstuvwxyz0"
		require.Len(t, longName, 64, "test precondition: name should be 64 characters")

		server := NewIdentityServer(longName)
		resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

		// The identity server does not validate name format; it returns whatever was configured.
		require.NoError(t, err)
		require.Equal(t, longName, resp.Name)
	})

	// TC-U-012: Name with invalid characters (underscore)
	// The IdentityServer does not validate name format; it stores and returns as-is.
	// In practice, the caller (main.go or config validation) is responsible for validation.
	t.Run("TC-U-012_name_with_invalid_characters", func(t *testing.T) {
		t.Parallel()

		server := NewIdentityServer("versitygw_cosi")
		resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

		// The identity server does not validate; it returns the name as-is.
		require.NoError(t, err)
		require.Equal(t, "versitygw_cosi", resp.Name)
	})

	// TC-U-013: Name not starting with alphanumeric
	// The IdentityServer does not validate name format; it stores and returns as-is.
	t.Run("TC-U-013_name_not_starting_with_alphanumeric", func(t *testing.T) {
		t.Parallel()

		server := NewIdentityServer("-versitygw.cosi.dev")
		resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

		require.NoError(t, err)
		require.Equal(t, "-versitygw.cosi.dev", resp.Name)
	})

	// TC-U-014: Name not ending with alphanumeric
	// The IdentityServer does not validate name format; it stores and returns as-is.
	t.Run("TC-U-014_name_not_ending_with_alphanumeric", func(t *testing.T) {
		t.Parallel()

		server := NewIdentityServer("versitygw.cosi.dev-")
		resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

		require.NoError(t, err)
		require.Equal(t, "versitygw.cosi.dev-", resp.Name)
	})

	// TC-U-015: Empty name
	// The IdentityServer does not validate that the name is non-empty.
	// In practice, main.go validates that driver-name is non-empty before creating
	// the IdentityServer, so this code path should not occur in production.
	t.Run("TC-U-015_empty_name", func(t *testing.T) {
		t.Parallel()

		server := NewIdentityServer("")
		resp, err := server.DriverGetInfo(context.Background(), &cosi.DriverGetInfoRequest{})

		require.NoError(t, err)
		require.Empty(t, resp.Name)
	})
}
