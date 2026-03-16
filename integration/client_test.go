package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	require.ErrorIs(t, err, auth.ErrUserExists, "expected ErrUserExists, got: %v", err)
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

func TestPutBucketPolicyMerge(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "merge-policy")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "mergeuser1", "secret1234567890", "user")
	require.NoError(t, err)
	err = client.CreateUser(ctx, "mergeuser2", "secret1234567890", "user")
	require.NoError(t, err)

	// Add first principal
	err = client.PutBucketPolicy(ctx, "merge-policy", "mergeuser1")
	require.NoError(t, err)

	// Add second principal (should merge, not overwrite)
	err = client.PutBucketPolicy(ctx, "merge-policy", "mergeuser2")
	require.NoError(t, err)

	// Both users should have access
	u1 := newUserS3Client(t, gw.Endpoint, "mergeuser1", "secret1234567890")
	_, err = u1.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("merge-policy"),
		Key:    aws.String("u1.txt"),
		Body:   strings.NewReader("user1"),
	})
	require.NoError(t, err)

	u2 := newUserS3Client(t, gw.Endpoint, "mergeuser2", "secret1234567890")
	_, err = u2.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("merge-policy"),
		Key:    aws.String("u2.txt"),
		Body:   strings.NewReader("user2"),
	})
	require.NoError(t, err)
}

func TestRemoveBucketPolicyPrincipal(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "remove-principal")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "rmuser1", "secret1234567890", "user")
	require.NoError(t, err)
	err = client.CreateUser(ctx, "rmuser2", "secret1234567890", "user")
	require.NoError(t, err)

	// Grant both users
	err = client.PutBucketPolicy(ctx, "remove-principal", "rmuser1")
	require.NoError(t, err)
	err = client.PutBucketPolicy(ctx, "remove-principal", "rmuser2")
	require.NoError(t, err)

	// Remove first user's principal
	err = client.RemoveBucketPolicyPrincipal(ctx, "remove-principal", "rmuser1")
	require.NoError(t, err)

	// Second user should still have access
	u2 := newUserS3Client(t, gw.Endpoint, "rmuser2", "secret1234567890")
	_, err = u2.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("remove-principal"),
		Key:    aws.String("test.txt"),
		Body:   strings.NewReader("data"),
	})
	require.NoError(t, err)
}

func TestPutBucketPolicyDuplicate(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "dup-policy")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "dupuser", "secret1234567890", "user")
	require.NoError(t, err)

	// Add same principal twice — should be idempotent
	err = client.PutBucketPolicy(ctx, "dup-policy", "dupuser")
	require.NoError(t, err)
	err = client.PutBucketPolicy(ctx, "dup-policy", "dupuser")
	require.NoError(t, err)

	// User should still have access (no duplicate entry issues)
	u := newUserS3Client(t, gw.Endpoint, "dupuser", "secret1234567890")
	_, err = u.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("dup-policy"),
		Key:    aws.String("test.txt"),
		Body:   strings.NewReader("data"),
	})
	require.NoError(t, err)
}

func TestRemoveBucketPolicyPrincipalNoPolicy(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "no-policy")
	require.NoError(t, err)

	// Remove from bucket with no policy — should be idempotent (no error)
	err = client.RemoveBucketPolicyPrincipal(ctx, "no-policy", "nonexistent")
	require.NoError(t, err)
}

func TestRemoveBucketPolicyPrincipalNonExistent(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "rm-nonexist")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "keepuser", "secret1234567890", "user")
	require.NoError(t, err)

	err = client.PutBucketPolicy(ctx, "rm-nonexist", "keepuser")
	require.NoError(t, err)

	// Remove a principal that doesn't exist in the policy — should not affect existing
	err = client.RemoveBucketPolicyPrincipal(ctx, "rm-nonexist", "ghost")
	require.NoError(t, err)

	// Original user should still have access
	u := newUserS3Client(t, gw.Endpoint, "keepuser", "secret1234567890")
	_, err = u.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("rm-nonexist"),
		Key:    aws.String("test.txt"),
		Body:   strings.NewReader("data"),
	})
	require.NoError(t, err)
}

func TestRemoveBucketPolicyLastPrincipal(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	ctx := context.Background()

	err := client.CreateBucket(ctx, "remove-last")
	require.NoError(t, err)

	err = client.CreateUser(ctx, "lastuser", "secret1234567890", "user")
	require.NoError(t, err)

	err = client.PutBucketPolicy(ctx, "remove-last", "lastuser")
	require.NoError(t, err)

	// Remove the only principal — policy should be deleted
	err = client.RemoveBucketPolicyPrincipal(ctx, "remove-last", "lastuser")
	require.NoError(t, err)

	// User should no longer have access
	u := newUserS3Client(t, gw.Endpoint, "lastuser", "secret1234567890")
	_, err = u.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("remove-last"),
		Key:    aws.String("test.txt"),
		Body:   strings.NewReader("data"),
	})
	require.Error(t, err)
}

func containsUser(users []auth.Account, access string) bool {
	for _, u := range users {
		if u.Access == access {
			return true
		}
	}
	return false
}
