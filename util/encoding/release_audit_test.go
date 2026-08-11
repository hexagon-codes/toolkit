package encoding

import (
	"errors"
	"testing"
)

func TestJoinURLPreservesQueryAndFragment(t *testing.T) {
	got, err := JoinURL("https://example.com/api?token=secret#section", "v1", "users")
	if err != nil {
		t.Fatalf("JoinURL() error: %v", err)
	}
	want := "https://example.com/api/v1/users?token=secret#section"
	if got != want {
		t.Fatalf("JoinURL() = %q, want %q", got, want)
	}
}

func TestJoinURLRejectsTraversal(t *testing.T) {
	for _, path := range []string{"../admin", "%2e%2e/admin", "users/./profile"} {
		t.Run(path, func(t *testing.T) {
			_, err := JoinURL("https://example.com/api/v1", path)
			if !errors.Is(err, ErrUnsafeURLPath) {
				t.Fatalf("JoinURL() error = %v, want ErrUnsafeURLPath", err)
			}
		})
	}
}

func TestJoinURLValidatesBaseWithoutAdditionalPaths(t *testing.T) {
	if _, err := JoinURL("https://example.com/%zz"); err == nil {
		t.Fatal("JoinURL accepted a malformed base URL")
	}
}

func TestJoinURLDoesNotTreatPathQueryAsBaseQuery(t *testing.T) {
	got, err := JoinURL("https://example.com/api", "users?admin=true")
	if err != nil {
		t.Fatalf("JoinURL() error: %v", err)
	}
	want := "https://example.com/api/users%3Fadmin=true"
	if got != want {
		t.Fatalf("JoinURL() = %q, want %q", got, want)
	}
}
