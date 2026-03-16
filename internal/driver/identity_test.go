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
