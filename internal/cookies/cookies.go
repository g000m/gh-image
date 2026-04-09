package cookies

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/brave"
	_ "github.com/browserutils/kooky/browser/chrome"
	_ "github.com/browserutils/kooky/browser/chromium"
	_ "github.com/browserutils/kooky/browser/edge"
)

const onePasswordCookieRefEnv = "GH_IMAGE_OP_COOKIE_REF"

var opRead = readOP

// GetGitHubSession returns the user_session cookie for github.com.
// It first checks 1Password via the op CLI, then falls back to Chrome, Brave,
// Edge, and Chromium (via kooky's registered finders).
func GetGitHubSession() (*http.Cookie, error) {
	opCookie, opErr := getGitHubSessionFrom1Password()
	if opErr == nil {
		return opCookie, nil
	}

	browserCookie, browserErr := getGitHubSessionFromBrowser()
	if browserErr == nil {
		return browserCookie, nil
	}

	return nil, fmt.Errorf("%v; fallback failed: %w", opErr, browserErr)
}

func getGitHubSessionFrom1Password() (*http.Cookie, error) {
	ref := strings.TrimSpace(os.Getenv(onePasswordCookieRefEnv))
	if ref == "" {
		return nil, fmt.Errorf("%s is not set", onePasswordCookieRefEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	value, err := opRead(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("reading GitHub session cookie from 1Password: %w", err)
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("reading GitHub session cookie from 1Password: empty secret")
	}

	return &http.Cookie{
		Name:     "user_session",
		Value:    value,
		Domain:   "github.com",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
	}, nil
}

func readOP(ctx context.Context, ref string) (string, error) {
	output, err := exec.CommandContext(ctx, "op", "read", ref).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return "", fmt.Errorf("%w: %s", err, message)
		}
		return "", err
	}
	return string(output), nil
}

func getGitHubSessionFromBrowser() (*http.Cookie, error) {
	ctx := context.Background()
	cookies, err := kooky.ReadCookies(ctx,
		kooky.Valid,
		kooky.DomainHasSuffix("github.com"),
		kooky.Name("user_session"),
	)

	// kooky returns errors for browsers/profiles that don't exist alongside
	// cookies from ones that do. Only fail if we got zero cookies.
	if len(cookies) > 0 {
		return &cookies[0].Cookie, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading browser cookies: %w", err)
	}

	return nil, fmt.Errorf("no github.com user_session cookie found in any supported browser — are you logged into GitHub?")
}
