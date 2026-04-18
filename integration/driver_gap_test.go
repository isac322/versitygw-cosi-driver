package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/versity/versitygw/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	cosi "sigs.k8s.io/container-object-storage-interface-spec"

	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

// TC-I-011: Created user has correct role ("user")
func TestGrantedUserHasUserRole(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "role-test"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "role-test",
		Name:               "role-check",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	users, err := client.ListUsers(ctx)
	require.NoError(t, err)

	var found bool
	for _, u := range users {
		if u.Access == grantResp.AccountId {
			found = true
			require.Equal(t, auth.RoleUser, u.Role, "granted user must have 'user' role, not admin")
			break
		}
	}
	require.True(t, found, "granted user %q not found in ListUsers", grantResp.AccountId)
}

// TC-I-024 + TC-I-025: Bucket policy has correct principal (access key ID, not ARN) and correct actions
func TestBucketPolicyAfterGrant(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "policy-verify"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "policy-verify",
		Name:               "policy-user",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	// Read policy directly
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	policy, err := client.GetBucketPolicy(ctx, "policy-verify")
	require.NoError(t, err)
	require.NotNil(t, policy, "policy should exist after grant")

	// Verify principal is the raw access key ID (not an ARN)
	require.NotEmpty(t, policy.Statement)
	stmt := policy.Statement[0]
	principals := stmt.Principal["AWS"]
	require.Contains(t, principals, grantResp.AccountId,
		"policy principal must contain the access key ID")

	// Verify it's NOT an ARN
	for _, p := range principals {
		require.NotContains(t, p, "arn:aws:iam",
			"VersityGW uses access key IDs as principals, not ARNs")
	}

	// Verify correct resources
	require.Contains(t, stmt.Resource, "arn:aws:s3:::policy-verify")
	require.Contains(t, stmt.Resource, "arn:aws:s3:::policy-verify/*")

	// Verify action
	require.Equal(t, "s3:*", stmt.Action)
}

// TC-I-034: Revoke one of multiple — policy updated correctly
func TestPolicyAfterPartialRevoke(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "partial-revoke"})
	require.NoError(t, err)

	// Grant two users
	resp1, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "partial-revoke",
		Name:               "user1",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	resp2, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "partial-revoke",
		Name:               "user2",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	// Revoke user1
	_, err = srv.DriverRevokeBucketAccess(ctx, &cosi.DriverRevokeBucketAccessRequest{
		BucketId:  "partial-revoke",
		AccountId: resp1.AccountId,
	})
	require.NoError(t, err)

	// Verify policy
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	policy, err := client.GetBucketPolicy(ctx, "partial-revoke")
	require.NoError(t, err)
	require.NotNil(t, policy, "policy should still exist")

	policyJSON, _ := json.Marshal(policy)
	require.NotContains(t, string(policyJSON), resp1.AccountId,
		"revoked user's principal should be removed from policy")
	require.Contains(t, string(policyJSON), resp2.AccountId,
		"remaining user's principal should still be in policy")
}

// TC-I-035: Revoke all → policy fully removed
func TestPolicyRemovedAfterAllRevoked(t *testing.T) {
	t.Parallel()
	srv, gw := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "all-revoke"})
	require.NoError(t, err)

	resp1, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "all-revoke",
		Name:               "u1",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	resp2, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "all-revoke",
		Name:               "u2",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	// Revoke both
	_, err = srv.DriverRevokeBucketAccess(ctx, &cosi.DriverRevokeBucketAccessRequest{
		BucketId: "all-revoke", AccountId: resp1.AccountId,
	})
	require.NoError(t, err)

	_, err = srv.DriverRevokeBucketAccess(ctx, &cosi.DriverRevokeBucketAccessRequest{
		BucketId: "all-revoke", AccountId: resp2.AccountId,
	})
	require.NoError(t, err)

	// Policy should be completely removed
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	policy, err := client.GetBucketPolicy(ctx, "all-revoke")
	require.NoError(t, err)
	require.Nil(t, policy, "policy should be nil (deleted) after all principals revoked")
}

// TC-I-054: Protocol response has S3 with correct fields (BucketInfo)
func TestCreateBucketResponseHasBucketInfo(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	resp, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "info-test"})
	require.NoError(t, err)

	require.NotNil(t, resp.BucketInfo, "DriverCreateBucketResponse must include BucketInfo")

	s3Info := resp.BucketInfo.GetS3()
	require.NotNil(t, s3Info, "BucketInfo must have S3 protocol")
	require.Equal(t, "us-east-1", s3Info.Region)
	require.Equal(t, cosi.S3SignatureVersion_S3V4, s3Info.SignatureVersion)
}

// TC-I-061: Grant with IAM auth type → INVALID_ARGUMENT
func TestGrantIAMAuthReturnsInvalidArgument(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "iam-reject"})
	require.NoError(t, err)

	_, err = srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "iam-reject",
		Name:               "iam-user",
		AuthenticationType: cosi.AuthenticationType_IAM,
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "expected gRPC status error")
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Contains(t, st.Message(), "IAM")
}

// TC-I-067: Delete bucket with active access grants
func TestDeleteBucketWithActiveAccess(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "active-del"})
	require.NoError(t, err)

	_, err = srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "active-del",
		Name:               "active-user",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)

	// Delete bucket without revoking access first.
	// The driver does not check for active access; it delegates to S3 DeleteBucket.
	// S3 will succeed since the bucket is empty (policies don't prevent delete).
	_, err = srv.DriverDeleteBucket(ctx, &cosi.DriverDeleteBucketRequest{BucketId: "active-del"})
	require.NoError(t, err)
}
