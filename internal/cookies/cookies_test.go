package cookies

import (
	"context"
	"errors"
	"testing"
)

func TestGetGitHubSessionFrom1PasswordRequiresRef(t *testing.T) {
	t.Setenv(onePasswordCookieRefEnv, "")

	_, err := getGitHubSessionFrom1Password()
	if err == nil {
		t.Fatal("expected missing ref error")
	}
}

func TestGetGitHubSessionFrom1PasswordReadsCookie(t *testing.T) {
	t.Setenv(onePasswordCookieRefEnv, "op://vault/item/user_session")

	original := opRead
	t.Cleanup(func() {
		opRead = original
	})

	opRead = func(_ context.Context, ref string) (string, error) {
		if ref != "op://vault/item/user_session" {
			t.Fatalf("unexpected ref: %s", ref)
		}
		return " cookie-value \n", nil
	}

	cookie, err := getGitHubSessionFrom1Password()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cookie.Name != "user_session" {
		t.Fatalf("unexpected cookie name: %s", cookie.Name)
	}
	if cookie.Value != "cookie-value" {
		t.Fatalf("unexpected cookie value: %q", cookie.Value)
	}
	if cookie.Domain != "github.com" {
		t.Fatalf("unexpected cookie domain: %s", cookie.Domain)
	}
}

func TestGetGitHubSessionFrom1PasswordRejectsEmptySecret(t *testing.T) {
	t.Setenv(onePasswordCookieRefEnv, "op://vault/item/user_session")

	original := opRead
	t.Cleanup(func() {
		opRead = original
	})

	opRead = func(context.Context, string) (string, error) {
		return " \n", nil
	}

	_, err := getGitHubSessionFrom1Password()
	if err == nil {
		t.Fatal("expected empty secret error")
	}
}

func TestGetGitHubSessionFrom1PasswordReturnsOpError(t *testing.T) {
	t.Setenv(onePasswordCookieRefEnv, "op://vault/item/user_session")

	original := opRead
	t.Cleanup(func() {
		opRead = original
	})

	opRead = func(context.Context, string) (string, error) {
		return "", errors.New("op failed")
	}

	_, err := getGitHubSessionFrom1Password()
	if err == nil {
		t.Fatal("expected op error")
	}
}
