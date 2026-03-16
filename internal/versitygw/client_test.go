package versitygw

import (
	"encoding/json"
	"testing"

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

	stmt := bucketPolicyStmt{
		Effect:    "Allow",
		Principal: policyPrincipal{"AWS": {"user1", "user2"}},
		Action:    "s3:*",
		Resource:  []string{"arn:aws:s3:::bucket", "arn:aws:s3:::bucket/*"},
	}

	data, err := json.Marshal(stmt)
	require.NoError(t, err)

	var decoded bucketPolicyStmt
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, stmt, decoded)
}

func TestBucketPolicyStmtUnmarshalSinglePrincipal(t *testing.T) {
	t.Parallel()

	// S3 may return a single principal as a string instead of an array
	input := `{"Effect":"Allow","Principal":{"AWS":"user1"},"Action":"s3:*","Resource":["arn:aws:s3:::bucket"]}`

	var stmt bucketPolicyStmt
	err := json.Unmarshal([]byte(input), &stmt)
	require.NoError(t, err)
	assert.Equal(t, policyPrincipal{"AWS": {"user1"}}, stmt.Principal)
}
