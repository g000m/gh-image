package cookies

import (
	"path/filepath"
	"testing"
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
