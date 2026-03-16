package versitygw

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyPrincipalUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    policyPrincipal
		wantErr bool
	}{
		{
			name:  "array_single",
			input: `{"AWS": ["user1"]}`,
			want:  policyPrincipal{"AWS": {"user1"}},
		},
		{
			name:  "array_multiple",
			input: `{"AWS": ["user1", "user2", "user3"]}`,
			want:  policyPrincipal{"AWS": {"user1", "user2", "user3"}},
		},
		{
			name:  "string_single",
			input: `{"AWS": "user1"}`,
			want:  policyPrincipal{"AWS": {"user1"}},
		},
		{
			name:  "string_arn",
			input: `{"AWS": "arn:aws:iam::123456789012:user/Alice"}`,
			want:  policyPrincipal{"AWS": {"arn:aws:iam::123456789012:user/Alice"}},
		},
		{
			name:  "multiple_types_array",
			input: `{"AWS": ["user1"], "Service": ["s3.amazonaws.com"]}`,
			want:  policyPrincipal{"AWS": {"user1"}, "Service": {"s3.amazonaws.com"}},
		},
		{
			name:  "multiple_types_string",
			input: `{"AWS": "user1", "Service": "s3.amazonaws.com"}`,
			want:  policyPrincipal{"AWS": {"user1"}, "Service": {"s3.amazonaws.com"}},
		},
		{
			name:  "empty_object",
			input: `{}`,
			want:  policyPrincipal{},
		},
		{
			name:  "empty_array",
			input: `{"AWS": []}`,
			want:  policyPrincipal{"AWS": {}},
		},
		{
			name:    "plain_string",
			input:   `"*"`,
			wantErr: true,
		},
		{
			name:    "number",
			input:   `123`,
			wantErr: true,
		},
		{
			name:    "array_of_numbers",
			input:   `[1, 2, 3]`,
			wantErr: true,
		},
		{
			name:    "nested_object",
			input:   `{"AWS": {"nested": "value"}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var p policyPrincipal
			err := json.Unmarshal([]byte(tt.input), &p)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, p)
		})
	}
}

func TestPolicyPrincipalMarshalJSON(t *testing.T) {
	t.Parallel()

	p := policyPrincipal{"AWS": {"user1", "user2"}}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	var roundtrip policyPrincipal
	err = json.Unmarshal(data, &roundtrip)
	require.NoError(t, err)
	assert.Equal(t, p, roundtrip)
}

func TestBucketPolicyStmtRoundtrip(t *testing.T) {
	t.Parallel()

	stmt := BucketPolicyStmt{
		Effect:    "Allow",
		Principal: policyPrincipal{"AWS": {"user1", "user2"}},
		Action:    "s3:*",
		Resource:  []string{"arn:aws:s3:::bucket", "arn:aws:s3:::bucket/*"},
	}

	data, err := json.Marshal(stmt)
	require.NoError(t, err)

	var decoded BucketPolicyStmt
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, stmt, decoded)
}

func TestBucketPolicyStmtUnmarshalSinglePrincipal(t *testing.T) {
	t.Parallel()

	// S3 may return a single principal as a string instead of an array
	input := `{"Effect":"Allow","Principal":{"AWS":"user1"},"Action":"s3:*","Resource":["arn:aws:s3:::bucket"]}`

	var stmt BucketPolicyStmt
	err := json.Unmarshal([]byte(input), &stmt)
	require.NoError(t, err)
	assert.Equal(t, policyPrincipal{"AWS": {"user1"}}, stmt.Principal)
}

func TestParseAdminError(t *testing.T) {
	t.Parallel()

	c := &Client{}

	tests := []struct {
		name       string
		statusCode int
		body       string
		operation  string
		wantSub    string
	}{
		{
			name:       "xml_error_with_code_and_message",
			statusCode: http.StatusBadRequest,
			body:       `<Error><Code>InvalidBucketName</Code><Message>bucket name is invalid</Message></Error>`,
			operation:  "create bucket",
			wantSub:    "InvalidBucketName: bucket name is invalid",
		},
		{
			name:       "xml_error_includes_operation",
			statusCode: http.StatusConflict,
			body:       `<Error><Code>BucketAlreadyExists</Code><Message>exists</Message></Error>`,
			operation:  "create bucket",
			wantSub:    "create bucket: BucketAlreadyExists",
		},
		{
			name:       "non_xml_body_shows_status",
			statusCode: http.StatusInternalServerError,
			body:       "internal server error",
			operation:  "list users",
			wantSub:    "unexpected status 500",
		},
		{
			name:       "non_xml_body_includes_raw_body",
			statusCode: http.StatusInternalServerError,
			body:       "something went wrong",
			operation:  "list users",
			wantSub:    "something went wrong",
		},
		{
			name:       "empty_body",
			statusCode: http.StatusForbidden,
			body:       "",
			operation:  "delete user",
			wantSub:    "unexpected status 403",
		},
		{
			name:       "xml_without_code_falls_back",
			statusCode: http.StatusBadGateway,
			body:       `<Error><Message>no code field</Message></Error>`,
			operation:  "test op",
			wantSub:    "unexpected status 502",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			err := c.parseAdminError(resp, tt.operation)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestAdminRequest(t *testing.T) {
	t.Parallel()

	t.Run("sends_patch_with_sigv4_headers", func(t *testing.T) {
		t.Parallel()
		var gotMethod string
		var gotHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/list-users", nil, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.MethodPatch, gotMethod)
		assert.NotEmpty(t, gotHeaders.Get("Authorization"))
		assert.NotEmpty(t, gotHeaders.Get("X-Amz-Content-Sha256"))
		assert.NotEmpty(t, gotHeaders.Get("X-Amz-Date"))
	})

	t.Run("includes_query_params", func(t *testing.T) {
		t.Parallel()
		var gotRawQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRawQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/delete-user", map[string]string{"access": "testuser"}, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Contains(t, gotRawQuery, "access=testuser")
	})

	t.Run("sends_body_with_content_type", func(t *testing.T) {
		t.Parallel()
		var gotContentType string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		body := []byte("<Account><Access>test</Access></Account>")
		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/create-user", nil, body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "application/xml", gotContentType)
		assert.Equal(t, body, gotBody)
	})

	t.Run("nil_body_omits_content_type", func(t *testing.T) {
		t.Parallel()
		var gotContentType string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/list-buckets", nil, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Empty(t, gotContentType)
	})
}

func newTestAdminClient(srv *httptest.Server) *Client {
	return &Client{
		adminEndpoint: srv.URL,
		creds:         aws.Credentials{AccessKeyID: "test", SecretAccessKey: "secret"},
		httpClient:    srv.Client(),
		region:        "us-east-1",
	}
}
