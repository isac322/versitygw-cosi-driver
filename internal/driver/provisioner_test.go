package driver

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/versity/versitygw/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockAPIError implements smithy.APIError for testing.
type mockAPIError struct {
	code    string
	message string
}

func (e *mockAPIError) Error() string                  { return fmt.Sprintf("%s: %s", e.code, e.message) }
func (e *mockAPIError) ErrorCode() string              { return e.code }
func (e *mockAPIError) ErrorMessage() string           { return e.message }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault  { return smithy.FaultUnknown }

func TestMapToGRPCError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		msg      string
		wantCode codes.Code
		wantMsg  string
	}{
		{
			name:     "user_exists",
			err:      auth.ErrUserExists,
			msg:      "create user",
			wantCode: codes.AlreadyExists,
			wantMsg:  "create user",
		},
		{
			name:     "no_such_user",
			err:      auth.ErrNoSuchUser,
			msg:      "delete user",
			wantCode: codes.NotFound,
			wantMsg:  "delete user",
		},
		{
			name:     "wrapped_user_exists",
			err:      fmt.Errorf("outer: %w", auth.ErrUserExists),
			msg:      "test",
			wantCode: codes.AlreadyExists,
		},
		{
			name:     "bucket_already_exists",
			err:      &mockAPIError{code: "BucketAlreadyExists", message: "bucket exists"},
			msg:      "create bucket",
			wantCode: codes.AlreadyExists,
		},
		{
			name:     "bucket_already_owned",
			err:      &mockAPIError{code: "BucketAlreadyOwnedByYou", message: "owned"},
			msg:      "create bucket",
			wantCode: codes.AlreadyExists,
		},
		{
			name:     "no_such_bucket",
			err:      &mockAPIError{code: "NoSuchBucket", message: "not found"},
			msg:      "delete bucket",
			wantCode: codes.NotFound,
		},
		{
			name:     "invalid_bucket_name",
			err:      &mockAPIError{code: "InvalidBucketName", message: "invalid"},
			msg:      "create bucket",
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "bucket_not_empty",
			err:      &mockAPIError{code: "BucketNotEmpty", message: "not empty"},
			msg:      "delete bucket",
			wantCode: codes.FailedPrecondition,
		},
		{
			name:     "unknown_api_error",
			err:      &mockAPIError{code: "InternalError", message: "server error"},
			msg:      "operation",
			wantCode: codes.Internal,
		},
		{
			name:     "context_canceled",
			err:      context.Canceled,
			msg:      "operation",
			wantCode: codes.Canceled,
		},
		{
			name:     "context_deadline_exceeded",
			err:      context.DeadlineExceeded,
			msg:      "operation",
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:     "wrapped_context_canceled",
			err:      fmt.Errorf("request failed: %w", context.Canceled),
			msg:      "test",
			wantCode: codes.Canceled,
		},
		{
			name:     "generic_error",
			err:      errors.New("something went wrong"),
			msg:      "operation",
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := mapToGRPCError(tt.err, tt.msg)
			require.Error(t, result)

			st, ok := status.FromError(result)
			require.True(t, ok, "expected gRPC status error")
			assert.Equal(t, tt.wantCode, st.Code())
			if tt.wantMsg != "" {
				assert.Contains(t, st.Message(), tt.wantMsg)
			}
		})
	}
}

func TestValidateBucketName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bucket  string
		wantErr bool
		errSub  string // substring expected in error message
	}{
		// Valid names
		{"min_length_3", "abc", false, ""},
		{"max_length_63", strings.Repeat("a", 63), false, ""},
		{"with_hyphens", "my-test-bucket", false, ""},
		{"starts_with_number", "0bucket", false, ""},
		{"ends_with_number", "bucket0", false, ""},
		{"all_digits", "123", false, ""},
		{"mixed", "a1-b2-c3", false, ""},

		// Invalid: length
		{"empty", "", true, "between 3 and 63"},
		{"too_short_1", "a", true, "between 3 and 63"},
		{"too_short_2", "ab", true, "between 3 and 63"},
		{"too_long_64", strings.Repeat("a", 64), true, "between 3 and 63"},
		{"too_long_100", strings.Repeat("a", 100), true, "between 3 and 63"},

		// Invalid: characters
		{"uppercase", "MyBucket", true, "invalid character"},
		{"underscore", "my_bucket", true, "invalid character"},
		{"period", "my.bucket", true, "invalid character"},
		{"space", "my bucket", true, "invalid character"},
		{"at_sign", "my@bucket", true, "invalid character"},
		{"exclamation", "my!bucket", true, "invalid character"},
		{"unicode", "mybücket", true, "invalid character"},

		// Invalid: start/end
		{"starts_with_hyphen", "-bucket", true, "must start with"},
		{"ends_with_hyphen", "bucket-", true, "must end with"},
		{"hyphen_only_3", "---", true, "must start with"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateBucketName(tt.bucket)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)

			// Verify it's a gRPC InvalidArgument error
			st, ok := status.FromError(err)
			require.True(t, ok, "expected gRPC status error")
			assert.Equal(t, codes.InvalidArgument, st.Code())

			if tt.errSub != "" {
				assert.Contains(t, st.Message(), tt.errSub)
			}
		})
	}
}

func TestGenerateSecretKey(t *testing.T) {
	t.Parallel()

	key, err := generateSecretKey()
	require.NoError(t, err)

	// 20 random bytes → 40 hex characters
	assert.Len(t, key, 40)

	_, err = hex.DecodeString(key)
	require.NoError(t, err)

	// Each call should produce a unique value
	key2, err := generateSecretKey()
	require.NoError(t, err)
	assert.NotEqual(t, key, key2)
}
