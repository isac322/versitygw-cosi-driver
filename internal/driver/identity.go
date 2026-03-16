// Package driver implements the COSI identity and provisioner servers.
package driver

import (
	"context"

	cosi "sigs.k8s.io/container-object-storage-interface-spec"
)

// IdentityServer implements the COSI IdentityServer interface.
type IdentityServer struct {
	driverName string
}

// NewIdentityServer creates a new IdentityServer with the given driver name.
func NewIdentityServer(driverName string) *IdentityServer {
	return &IdentityServer{driverName: driverName}
}

// DriverGetInfo returns the driver name.
func (s *IdentityServer) DriverGetInfo(_ context.Context, _ *cosi.DriverGetInfoRequest) (*cosi.DriverGetInfoResponse, error) {
	return &cosi.DriverGetInfoResponse{Name: s.driverName}, nil
}
