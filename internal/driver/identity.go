// Package driver implements the COSI identity and provisioner servers.
package driver

import (
	"context"

	cosi "sigs.k8s.io/container-object-storage-interface-spec"
)

const driverName = "versitygw.cosi.dev"

// IdentityServer implements the COSI IdentityServer interface.
type IdentityServer struct{}

// DriverGetInfo returns the driver name.
func (s *IdentityServer) DriverGetInfo(_ context.Context, _ *cosi.DriverGetInfoRequest) (*cosi.DriverGetInfoResponse, error) {
	return &cosi.DriverGetInfoResponse{Name: driverName}, nil
}
