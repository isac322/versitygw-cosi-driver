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
		BucketId: "crud-test",
		Name:     "crud-app",
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets

	// Create S3 client with granted credentials
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	// PutObject
	_, err = userS3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("crud-test"),
		Key:    aws.String("hello.txt"),
		Body:   strings.NewReader("world"),
	})
	require.NoError(t, err)

	// GetObject
	out, err := userS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("crud-test"),
		Key:    aws.String("hello.txt"),
	})
	require.NoError(t, err)
	body, err := io.ReadAll(out.Body)
	out.Body.Close()
	require.NoError(t, err)
	require.Equal(t, "world", string(body))

	// DeleteObject
	_, err = userS3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String("crud-test"),
		Key:    aws.String("hello.txt"),
	})
	require.NoError(t, err)

	// Verify deleted
	_, err = userS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("crud-test"),
		Key:    aws.String("hello.txt"),
	})
	require.Error(t, err)
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
		BucketId: "overwrite-test",
		Name:     "overwrite-app",
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
		BucketId: "large-test",
		Name:     "large-app",
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
	require.Equal(t, len(largeData), len(body))
}

func TestObjectListAfterCRUD(t *testing.T) {
	t.Parallel()
	gw := testutil.StartVersityGW(t)
	client := versitygw.NewClient(gw.Endpoint, gw.AdminEndpoint, gw.AccessKey, gw.SecretKey)
	srv := driver.NewProvisionerServer(client, gw.Endpoint, "us-east-1")
	ctx := context.Background()

	_, err := srv.DriverCreateBucket(ctx, &cosi.DriverCreateBucketRequest{Name: "list-test"})
	require.NoError(t, err)

	grantResp, err := srv.DriverGrantBucketAccess(ctx, &cosi.DriverGrantBucketAccessRequest{
		BucketId: "list-test",
		Name:     "list-app",
	})
	require.NoError(t, err)
	creds := grantResp.Credentials["s3"].Secrets
	userS3 := newUserS3Client(t, creds["endpoint"], creds["accessKeyID"], creds["accessSecretKey"])

	// Create objects
	for _, key := range []string{"a.txt", "b.txt", "c.txt"} {
		_, err = userS3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("list-test"),
			Key:    aws.String(key),
			Body:   strings.NewReader("content"),
		})
		require.NoError(t, err)
	}

	// List objects
	listResp, err := userS3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String("list-test"),
	})
	require.NoError(t, err)
	require.Equal(t, int32(3), *listResp.KeyCount)

	// Delete one
	_, err = userS3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String("list-test"),
		Key:    aws.String("b.txt"),
	})
	require.NoError(t, err)

	// List again
	listResp, err = userS3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String("list-test"),
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), *listResp.KeyCount)

	keys := make([]string, len(listResp.Contents))
	for i, obj := range listResp.Contents {
		keys[i] = *obj.Key
	}
	require.Contains(t, keys, "a.txt")
	require.Contains(t, keys, "c.txt")
	require.NotContains(t, keys, "b.txt")
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
		BucketId: "revoke-s3-test",
		Name:     "revoke-app",
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

	// Access should be denied (user is deleted, so any request should fail)
	_, err = userS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("revoke-s3-test"),
		Key:    aws.String("test.txt"),
	})
	require.Error(t, err)
}
