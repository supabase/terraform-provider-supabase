package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestNotFoundErr_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *NotFoundErr
		want string
	}{
		{
			name: "resource and id",
			err:  NewNotFoundErr("third-party auth integration", "abc"),
			want: `third-party auth integration "abc" not found`,
		},
		{
			name: "resource only",
			err:  NewNotFoundErr("third-party auth integration", ""),
			want: "third-party auth integration not found",
		},
		{
			name: "id only",
			err:  NewNotFoundErr("", "abc"),
			want: `"abc" not found`,
		},
		{
			name: "neither",
			err:  NewNotFoundErr("", ""),
			want: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewNotFoundErr(t *testing.T) {
	err := NewNotFoundErr("thing", "123")
	if err.Resource != "thing" || err.ID != "123" {
		t.Errorf("NewNotFoundErr() = %+v, want Resource %q and ID %q", err, "thing", "123")
	}
}

func TestNotFound(t *testing.T) {
	base := NewNotFoundErr("thing", "123")

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unrelated error", err: errors.New("boom"), want: false},
		{name: "direct", err: base, want: true},
		{name: "wrapped with %w", err: fmt.Errorf("reading thing: %w", base), want: true},
		{name: "wrapped twice", err: fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", base)), want: true},
		{name: "joined", err: errors.Join(errors.New("boom"), base), want: true},
		{name: "wrapped with %v loses type", err: fmt.Errorf("reading thing: %v", base), want: false},
		{name: "unrelated wrapped", err: fmt.Errorf("outer: %w", errors.New("boom")), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NotFound(tt.err); got != tt.want {
				t.Errorf("NotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
