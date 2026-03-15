package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	cosi "sigs.k8s.io/container-object-storage-interface-spec"

	"github.com/isac322/versitygw-cosi-driver/integration/testutil"
	"github.com/isac322/versitygw-cosi-driver/internal/driver"
	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

func newTestServer(t *testing.T) (*driver.ProvisionerServer, *testutil.VersityGWInstance) {
	t.Helper()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	return srv, gw
}

func TestDriverCreateBucket(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	resp, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{
		Name: "driver-create",
	})
	require.NoError(t, err)
	require.Equal(t, "driver-create", resp.BucketId)
}

func TestDriverCreateBucketIdempotent(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "driver-idem"})
	require.NoError(t, err)

	resp, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "driver-idem"})
	require.NoError(t, err)
	require.Equal(t, "driver-idem", resp.BucketId)
}

func TestDriverDeleteBucket(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "driver-del"})
	require.NoError(t, err)

	_, err = srv.DriverDeleteBucket(ctx, &cosi.DriverDeleteBucketRequest{BucketId: "driver-del"})
	require.NoError(t, err)
}

func TestDriverDeleteBucketIdempotent(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverDeleteBucket(ctx, &cosi.DriverDeleteBucketRequest{BucketId: "nonexistent-bucket"})
	require.NoError(t, err)
}

func TestDriverGrantBucketAccess(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "grant-test"})
	require.NoError(t, err)

	resp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId: "grant-test",
		Name:     "test-access",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccountId)

	creds := resp.Credentials["s3"]
	require.NotNil(t, creds)
	require.NotEmpty(t, creds.Secrets["accessKeyID"])
	require.NotEmpty(t, creds.Secrets["accessSecretKey"])
	require.Equal(t, gw.Endpoint, creds.Secrets["endpoint"])
	require.Equal(t, "us-east-1", creds.Secrets["region"])

	// Verify user was created
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	users, err := client.ListUsers(ctx)
	require.NoError(t, err)
	require.True(t, containsUser(users, resp.AccountId))
}

func TestDriverRevokeBucketAccess(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "revoke-test"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId: "revoke-test",
		Name:     "test-revoke",
	})
	require.NoError(t, err)

	_, err = srv.DriverRevokeBucketAccess(ctx, &cosi.DriverRevokeBucketAccessRequest{
		BucketId:  "revoke-test",
		AccountId: grantResp.AccountId,
	})
	require.NoError(t, err)

	// Verify user was deleted
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	users, err := client.ListUsers(ctx)
	require.NoError(t, err)
	require.False(t, containsUser(users, grantResp.AccountId))
}

func TestDriverGrantThenRevoke(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	// Create bucket
	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "flow-test"})
	require.NoError(t, err)

	// Grant access
	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId: "flow-test",
		Name:     "test-flow",
	})
	require.NoError(t, err)
	require.NotEmpty(t, grantResp.AccountId)
	require.NotNil(t, grantResp.Credentials["s3"])

	// Revoke access
	_, err = srv.DriverRevokeBucketAccess(ctx, &cosi.DriverRevokeBucketAccessRequest{
		BucketId:  "flow-test",
		AccountId: grantResp.AccountId,
	})
	require.NoError(t, err)

	// Verify user gone
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	users, err := client.ListUsers(ctx)
	require.NoError(t, err)
	require.False(t, containsUser(users, grantResp.AccountId))

	// Delete bucket
	_, err = srv.DriverDeleteBucket(ctx, &cosi.DriverDeleteBucketRequest{BucketId: "flow-test"})
	require.NoError(t, err)
}
