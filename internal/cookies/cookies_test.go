package cookies

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/browserutils/kooky"
)

func TestInferBrowserFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{path: "/Users/me/Library/Application Support/Google/Chrome/Profile 1/Cookies", want: "chrome"},
		{path: "/Users/me/Library/Application Support/BraveSoftware/Brave-Browser/Default/Cookies", want: "brave"},
		{path: "/Users/me/Library/Application Support/Microsoft Edge/Default/Cookies", want: "edge"},
		{path: "/Users/me/.config/chromium/Default/Cookies", want: "chromium"},
		{path: "/tmp/custom/cookies.sqlite", want: ""},
	}

	for _, tt := range tests {
		if got := inferBrowserFromPath(tt.path); got != tt.want {
			t.Fatalf("inferBrowserFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestRememberedSourceRoundTrip(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	orig := userConfigDir
	userConfigDir = func() (string, error) { return tempDir, nil }
	t.Cleanup(func() { userConfigDir = orig })

	want := Source{
		Browser:  "chrome",
		Profile:  "Profile 1",
		CookieDB: "/tmp/Cookies",
	}
	if err := saveRememberedSource(want); err != nil {
		t.Fatalf("saveRememberedSource returned error: %v", err)
	}

	got, ok, err := loadRememberedSource()
	if err != nil {
		t.Fatalf("loadRememberedSource returned error: %v", err)
	}
	if !ok {
		t.Fatalf("loadRememberedSource ok = false, want true")
	}
	if got != want {
		t.Fatalf("loadRememberedSource = %#v, want %#v", got, want)
	}

	if err := clearRememberedSource(); err != nil {
		t.Fatalf("clearRememberedSource returned error: %v", err)
	}

	if _, ok, err := loadRememberedSource(); err != nil || ok {
		t.Fatalf("post-clear load = (_, %v, %v), want (_, false, nil)", ok, err)
	}

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath returned error: %v", err)
	}
	if wantPath := filepath.Join(tempDir, configDirName, configFileName); path != wantPath {
		t.Fatalf("configPath = %q, want %q", path, wantPath)
	}
}

func TestFilterStores(t *testing.T) {
	t.Parallel()

	stores := []candidateStore{
		{source: Source{Browser: "chrome", Profile: "Default", CookieDB: "a"}},
		{source: Source{Browser: "chrome", Profile: "Profile 1", CookieDB: "b"}},
		{source: Source{Browser: "brave", Profile: "Default", CookieDB: "c"}},
	}

	got := filterStores(stores, Options{Browser: "chrome", Profile: "Profile 1"})
	if len(got) != 1 || got[0].source.CookieDB != "b" {
		t.Fatalf("filterStores returned %#v, want only cookie DB b", got)
	}
}

// setupTestSeams redirects userConfigDir to a temp dir and restores all
// package-level seam vars on test cleanup. It returns the temp dir path.
func setupTestSeams(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()

	origConfigDir := userConfigDir
	origDiscoverFn := discoverStoresFn
	origDirectFn := readFromDirectSourceFn

	userConfigDir = func() (string, error) { return tempDir, nil }

	t.Cleanup(func() {
		userConfigDir = origConfigDir
		discoverStoresFn = origDiscoverFn
		readFromDirectSourceFn = origDirectFn
	})

	return tempDir
}

// panicDiscoverFn replaces discoverStoresFn to fail the test if discovery
// is called — used to assert the happy path bypasses discovery.
func panicDiscoverFn(t *testing.T) func(context.Context) ([]candidateStore, error) {
	t.Helper()
	return func(_ context.Context) ([]candidateStore, error) {
		t.Error("discoverStores was called, but should have been bypassed on the happy path")
		return nil, fmt.Errorf("discoverStores unexpectedly called")
	}
}

// fakeDirectSourceFn returns a readFromDirectSourceFn that yields a fixed
// cookie when called, simulating a warm remembered-source hit.
func fakeDirectSourceFn(cookie *http.Cookie, source Source) func(context.Context, Source, []kooky.Filter) (*http.Cookie, Source, error) {
	return func(_ context.Context, _ Source, _ []kooky.Filter) (*http.Cookie, Source, error) {
		return cookie, source, nil
	}
}

// TestGetGitHubSession_HappyPath_SkipsDiscovery verifies that when a valid
// remembered source exists and no browser/profile overrides are provided,
// GetGitHubSession returns immediately via readFromDirectSource without ever
// invoking discoverStores (i.e. no second keychain prompt on macOS).
func TestGetGitHubSession_HappyPath_SkipsDiscovery(t *testing.T) {
	setupTestSeams(t)

	remembered := Source{
		Browser:  "chrome",
		Profile:  "Default",
		CookieDB: "/fake/path/to/Cookies",
	}
	if err := saveRememberedSource(remembered); err != nil {
		t.Fatalf("saveRememberedSource: %v", err)
	}

	wantCookie := &http.Cookie{Name: "user_session", Value: "tok_abc123"}

	// Inject fake direct-source reader that returns the expected cookie.
	readFromDirectSourceFn = fakeDirectSourceFn(wantCookie, remembered)
	// Inject a discovery fn that fails the test if called.
	discoverStoresFn = panicDiscoverFn(t)

	got, err := GetGitHubSession(Options{})
	if err != nil {
		t.Fatalf("GetGitHubSession returned unexpected error: %v", err)
	}
	if got == nil || got.Value != wantCookie.Value {
		t.Fatalf("GetGitHubSession cookie = %v, want value %q", got, wantCookie.Value)
	}
}

// TestGetGitHubSession_StaleRememberedSource_FallsBackToDiscovery verifies
// that when the remembered source's direct read fails (stale path, browser
// moved, etc.), GetGitHubSession falls back to store discovery.
func TestGetGitHubSession_StaleRememberedSource_FallsBackToDiscovery(t *testing.T) {
	setupTestSeams(t)

	remembered := Source{
		Browser:  "chrome",
		Profile:  "Default",
		CookieDB: "/nonexistent/path/Cookies",
	}
	if err := saveRememberedSource(remembered); err != nil {
		t.Fatalf("saveRememberedSource: %v", err)
	}

	// Direct-source read fails (stale).
	readFromDirectSourceFn = func(_ context.Context, _ Source, _ []kooky.Filter) (*http.Cookie, Source, error) {
		return nil, Source{}, fmt.Errorf("cookie DB not found")
	}

	discoverCalled := false
	wantCookie := &http.Cookie{Name: "user_session", Value: "tok_fresh"}
	freshSource := Source{Browser: "chrome", Profile: "Default", CookieDB: "/new/path/Cookies"}

	discoverStoresFn = func(_ context.Context) ([]candidateStore, error) {
		discoverCalled = true
		// Return a single fake store that will be used by the discovery path.
		// We need a real-ish candidateStore; since readFromStore calls
		// store.TraverseCookies we stub at the discoverStoresFn level and also
		// override readFromDirectSourceFn for the post-discovery remembered-source
		// branch. For simplicity return an empty set — the function will then
		// return "no cookie stores found" which is fine for this assertion.
		_ = wantCookie
		_ = freshSource
		return nil, nil
	}

	_, err := GetGitHubSession(Options{})
	// We expect an error here (no stores found) — what matters is discovery ran.
	if err == nil {
		t.Fatal("GetGitHubSession expected error (no stores), got nil")
	}
	if !discoverCalled {
		t.Fatal("expected discoverStores to be called on stale remembered source, but it was not")
	}
}

// TestGetGitHubSession_BrowserOverride_SkipsRememberedFastPath verifies that
// passing --browser causes the remembered-source fast path to be skipped and
// store discovery to run instead.
func TestGetGitHubSession_BrowserOverride_SkipsRememberedFastPath(t *testing.T) {
	setupTestSeams(t)

	remembered := Source{
		Browser:  "chrome",
		Profile:  "Default",
		CookieDB: "/fake/path/Cookies",
	}
	if err := saveRememberedSource(remembered); err != nil {
		t.Fatalf("saveRememberedSource: %v", err)
	}

	directCalled := false
	readFromDirectSourceFn = func(_ context.Context, _ Source, _ []kooky.Filter) (*http.Cookie, Source, error) {
		directCalled = true
		return nil, Source{}, fmt.Errorf("should not be reached on fast path")
	}

	discoverCalled := false
	discoverStoresFn = func(_ context.Context) ([]candidateStore, error) {
		discoverCalled = true
		return nil, nil // empty — triggers "no stores found" error, which is fine
	}

	_, err := GetGitHubSession(Options{Browser: "brave"})
	if err == nil {
		t.Fatal("GetGitHubSession expected error (no stores), got nil")
	}
	if directCalled {
		t.Fatal("readFromDirectSource fast path should NOT have been called when --browser is set")
	}
	if !discoverCalled {
		t.Fatal("discoverStores should have been called when --browser is set")
	}
}

// TestGetGitHubSession_ProfileOverride_SkipsRememberedFastPath verifies the
// same bypass behaviour when --profile is provided.
func TestGetGitHubSession_ProfileOverride_SkipsRememberedFastPath(t *testing.T) {
	setupTestSeams(t)

	remembered := Source{
		Browser:  "chrome",
		Profile:  "Default",
		CookieDB: "/fake/path/Cookies",
	}
	if err := saveRememberedSource(remembered); err != nil {
		t.Fatalf("saveRememberedSource: %v", err)
	}

	directCalled := false
	readFromDirectSourceFn = func(_ context.Context, _ Source, _ []kooky.Filter) (*http.Cookie, Source, error) {
		directCalled = true
		return nil, Source{}, fmt.Errorf("should not be reached on fast path")
	}

	discoverCalled := false
	discoverStoresFn = func(_ context.Context) ([]candidateStore, error) {
		discoverCalled = true
		return nil, nil
	}

	_, err := GetGitHubSession(Options{Profile: "Work"})
	if err == nil {
		t.Fatal("GetGitHubSession expected error (no stores), got nil")
	}
	if directCalled {
		t.Fatal("readFromDirectSource fast path should NOT have been called when --profile is set")
	}
	if !discoverCalled {
		t.Fatal("discoverStores should have been called when --profile is set")
	}
}

// TestGetGitHubSession_ForgetSource_SkipsRememberedFastPath verifies that
// --forget-cookie-source clears the remembered source before the fast path,
// causing the function to fall through to discovery.
func TestGetGitHubSession_ForgetSource_SkipsRememberedFastPath(t *testing.T) {
	setupTestSeams(t)

	remembered := Source{
		Browser:  "chrome",
		Profile:  "Default",
		CookieDB: "/fake/path/Cookies",
	}
	if err := saveRememberedSource(remembered); err != nil {
		t.Fatalf("saveRememberedSource: %v", err)
	}

	directCalled := false
	readFromDirectSourceFn = func(_ context.Context, _ Source, _ []kooky.Filter) (*http.Cookie, Source, error) {
		directCalled = true
		return nil, Source{}, fmt.Errorf("should not be called after forget")
	}

	discoverCalled := false
	discoverStoresFn = func(_ context.Context) ([]candidateStore, error) {
		discoverCalled = true
		return nil, nil
	}

	_, err := GetGitHubSession(Options{ForgetRememberedSource: true})
	if err == nil {
		t.Fatal("GetGitHubSession expected error (no stores), got nil")
	}
	if directCalled {
		t.Fatal("readFromDirectSource fast path should NOT be called after forget-cookie-source")
	}
	if !discoverCalled {
		t.Fatal("discoverStores should have been called after forget-cookie-source cleared the remembered source")
	}
}
