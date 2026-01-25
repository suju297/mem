package app

import (
	"testing"
	"time"
)

func TestParseStateUpdatedAt(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{
			name:  "rfc3339nano",
			value: "2024-01-02T03:04:05.123456789Z",
			want:  "2024-01-02T03:04:05.123456789Z",
			ok:    true,
		},
		{
			name:  "rfc3339",
			value: "2024-01-02T03:04:05Z",
			want:  "2024-01-02T03:04:05Z",
			ok:    true,
		},
		{
			name:  "empty",
			value: "",
			ok:    false,
		},
		{
			name:  "garbage",
			value: "not-a-time",
			ok:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseStateUpdatedAt(tc.value)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if got.UTC().Format(time.RFC3339Nano) != tc.want {
				t.Fatalf("got=%s want=%s", got.UTC().Format(time.RFC3339Nano), tc.want)
			}
		})
	}
}
