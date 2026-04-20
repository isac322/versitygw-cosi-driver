package integration

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	cosi "sigs.k8s.io/container-object-storage-interface-spec"

	"github.com/isac322/versitygw-cosi-driver/integration/testutil"
	"github.com/isac322/versitygw-cosi-driver/internal/driver"
	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

func newUserS3Client(t *testing.T, endpoint, accessKey, secretKey string) *s3.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	require.NoError(t, err)

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
}

func TestObjectCRUD(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	ctx := context.Background()

	// Create bucket via driver
	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "crud-test"})
	require.NoError(t, err)

	// Grant access
	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "crud-test",
		Name:               "crud-app",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets

	// Create S3 client with granted credentials
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	// TC-I-020: Granted credentials allow PutObject
	t.Run("TC-I-020_granted_credentials_allow_put_object", func(t *testing.T) {
		t.Parallel()
		_, err := userS3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("crud-test"),
			Key:    aws.String("hello.txt"),
			Body:   strings.NewReader("world"),
		})
		require.NoError(t, err, "PutObject should succeed with granted credentials")
	})

	// TC-I-021: Granted credentials allow GetObject
	t.Run("TC-I-021_granted_credentials_allow_get_object", func(t *testing.T) {
		t.Parallel()
		// Write an object first using admin to ensure it exists
		adminS3 := newUserS3Client(t, gw.Endpoint, gw.AccessKey, gw.SecretKey)
		_, err := adminS3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("crud-test"),
			Key:    aws.String("admin-written.txt"),
			Body:   strings.NewReader("admin-data"),
		})
		require.NoError(t, err)

		out, err := userS3.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String("crud-test"),
			Key:    aws.String("admin-written.txt"),
		})
		require.NoError(t, err, "GetObject should succeed with granted credentials")
		body, err := io.ReadAll(out.Body)
		out.Body.Close()
		require.NoError(t, err)
		require.Equal(t, "admin-data", string(body))
	})

	// TC-I-023: Granted credentials allow DeleteObject
	t.Run("TC-I-023_granted_credentials_allow_delete_object", func(t *testing.T) {
		t.Parallel()
		// Write then delete
		_, err := userS3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("crud-test"),
			Key:    aws.String("to-delete.txt"),
			Body:   strings.NewReader("temp"),
		})
		require.NoError(t, err)

		_, err = userS3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String("crud-test"),
			Key:    aws.String("to-delete.txt"),
		})
		require.NoError(t, err, "DeleteObject should succeed with granted credentials")
	})
}

func TestObjectOverwrite(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "overwrite-test"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "overwrite-test",
		Name:               "overwrite-app",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	// Write v1
	_, err = userS3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("overwrite-test"),
		Key:    aws.String("data.txt"),
		Body:   strings.NewReader("version1"),
	})
	require.NoError(t, err)

	// Overwrite with v2
	_, err = userS3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("overwrite-test"),
		Key:    aws.String("data.txt"),
		Body:   strings.NewReader("version2"),
	})
	require.NoError(t, err)

	// Verify v2
	out, err := userS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("overwrite-test"),
		Key:    aws.String("data.txt"),
	})
	require.NoError(t, err)
	body, _ := io.ReadAll(out.Body)
	out.Body.Close()
	require.Equal(t, "version2", string(body))
}

func TestObjectLargeFile(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "large-test"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "large-test",
		Name:               "large-app",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	// 1MB file
	largeData := strings.Repeat("x", 1024*1024)
	_, err = userS3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("large-test"),
		Key:    aws.String("large.bin"),
		Body:   strings.NewReader(largeData),
	})
	require.NoError(t, err)

	out, err := userS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("large-test"),
		Key:    aws.String("large.bin"),
	})
	require.NoError(t, err)
	body, _ := io.ReadAll(out.Body)
	out.Body.Close()
	require.Len(t, body, len(largeData))
}

// TC-I-022: Granted credentials allow ListBucket
func TestObjectListAfterCRUD(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "list-test"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "list-test",
		Name:               "list-app",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	t.Run("TC-I-022_granted_credentials_allow_list_bucket", func(t *testing.T) {
		t.Parallel()
		// Create objects
		for _, key := range []string{"a.txt", "b.txt", "c.txt"} {
			_, err := userS3.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String("list-test"),
				Key:    aws.String(key),
				Body:   strings.NewReader("content"),
			})
			require.NoError(t, err)
		}

		// ListObjectsV2 should succeed and return expected keys
		listResp, err := userS3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String("list-test"),
		})
		require.NoError(t, err, "ListObjectsV2 should succeed with granted credentials")
		require.Equal(t, int32(3), *listResp.KeyCount)

		keys := make([]string, len(listResp.Contents))
		for i, obj := range listResp.Contents {
			keys[i] = *obj.Key
		}
		require.Contains(t, keys, "a.txt")
		require.Contains(t, keys, "b.txt")
		require.Contains(t, keys, "c.txt")
	})
}

func TestRevokedAccessDenied(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "revoke-s3-test"})
	require.NoError(t, err)

	// Grant
	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId:           "revoke-s3-test",
		Name:               "revoke-app",
		AuthenticationType: cosi.AuthenticationType_Key,
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	// Write while access is granted
	_, err = userS3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("revoke-s3-test"),
		Key:    aws.String("test.txt"),
		Body:   strings.NewReader("data"),
	})
	require.NoError(t, err)

	// Revoke
	_, err = srv.DriverRevokeBucketAccess(ctx, &cosi.DriverRevokeBucketAccessRequest{
		BucketId:  "revoke-s3-test",
		AccountId: grantResp.AccountId,
	})
	require.NoError(t, err)

	// TC-I-030: Revoked credentials fail for S3 PutObject
	t.Run("TC-I-030_revoked_credentials_fail_put_object", func(t *testing.T) {
		t.Parallel()
		_, err := userS3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("revoke-s3-test"),
			Key:    aws.String("after-revoke.txt"),
			Body:   strings.NewReader("denied"),
		})
		require.Error(t, err, "PutObject should fail after revoke")
	})

	// TC-I-031: Revoked credentials fail for S3 GetObject
	t.Run("TC-I-031_revoked_credentials_fail_get_object", func(t *testing.T) {
		t.Parallel()
		_, err := userS3.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String("revoke-s3-test"),
			Key:    aws.String("test.txt"),
		})
		require.Error(t, err, "GetObject should fail after revoke")
	})

	// TC-I-032: Revoke removes principal from bucket policy
	t.Run("TC-I-032_revoke_removes_principal_from_policy", func(t *testing.T) {
		t.Parallel()
		// This was the only principal, so policy should not exist
		policy, err := client.GetBucketPolicy(ctx, "revoke-s3-test")
		require.NoError(t, err)
		require.Nil(t, policy, "policy should be nil (deleted) after the only principal is revoked")
	})
}
