package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/versity/versitygw/auth"

	"github.com/isac322/versitygw-cosi-driver/integration/testutil"
	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

func TestCreateUser(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateUser(ctx, "testuser", "testsecret123456", "user")
	require.NoError(t, err)

	users, err := client.ListUsers(ctx)
	require.NoError(t, err)
	require.True(t, containsUser(users, "testuser"), "expected user 'testuser' in list")
}

func TestCreateUserAlreadyExists(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateUser(ctx, "dupuser", "secret12345678", "user")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "dupuser", "secret12345678", "user")
	require.True(t, errors.Is(err, auth.ErrUserExists), "expected ErrUserExists, got: %v", err)
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateUser(ctx, "deluser", "secret12345678", "user")
	require.NoError(t, err)

	err = client.DeleteUser(ctx, "deluser")
	require.NoError(t, err)

	users, err := client.ListUsers(ctx)
	require.NoError(t, err)
	require.False(t, containsUser(users, "deluser"), "expected user 'deluser' to be deleted")
}

func TestDeleteUserNotFound(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	// versitygw returns 200 OK for deleting non-existent users (idempotent).
	err := client.DeleteUser(ctx, "nonexistent")
	require.NoError(t, err)
}

func TestListUsers(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	for _, name := range []string{"user1", "user2", "user3"} {
		err := client.CreateUser(ctx, name, "secret12345678", "user")
		require.NoError(t, err)
	}

	users, err := client.ListUsers(ctx)
	require.NoError(t, err)
	for _, name := range []string{"user1", "user2", "user3"} {
		require.True(t, containsUser(users, name), "expected user %q in list", name)
	}
}

func TestCreateBucket(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "test-bucket")
	require.NoError(t, err)

	// Idempotent: create again should succeed
	err = client.CreateBucket(ctx, "test-bucket")
	require.NoError(t, err)
}

func TestDeleteBucket(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "del-bucket")
	require.NoError(t, err)

	err = client.DeleteBucket(ctx, "del-bucket")
	require.NoError(t, err)

	// Idempotent: delete again should succeed
	err = client.DeleteBucket(ctx, "del-bucket")
	require.NoError(t, err)
}

func TestChangeBucketOwner(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "owner-test")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "newowner", "secret12345678", "user")
	require.NoError(t, err)

	err = client.ChangeBucketOwner(ctx, "owner-test", "newowner")
	require.NoError(t, err)

	buckets, err := client.ListBuckets(ctx)
	require.NoError(t, err)
	for _, b := range buckets {
		if b.Name == "owner-test" {
			require.Equal(t, "newowner", b.Owner)
			return
		}
	}
	t.Fatal("bucket 'owner-test' not found in list")
}

func TestListBuckets(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	for _, name := range []string{"list-b1", "list-b2"} {
		err := client.CreateBucket(ctx, name)
		require.NoError(t, err)
	}

	buckets, err := client.ListBuckets(ctx)
	require.NoError(t, err)

	names := make([]string, len(buckets))
	for i, b := range buckets {
		names[i] = b.Name
	}
	require.Contains(t, names, "list-b1")
	require.Contains(t, names, "list-b2")
}

func TestPutBucketPolicy(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "policy-test")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "policyuser", "secret12345678", "user")
	require.NoError(t, err)

	err = client.PutBucketPolicy(ctx, "policy-test", "policyuser")
	require.NoError(t, err)
}

func containsUser(users []auth.Account, access string) bool {
	for _, u := range users {
		if u.Access == access {
			return true
		}
	}
	return false
}
