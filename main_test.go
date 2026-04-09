package main

import "testing"

func TestParseArgsSelectionFlags(t *testing.T) {
	opts, err := parseArgs([]string{
		"one.png",
		"--repo", "owner/repo",
		"--browser=chrome",
		"--profile", "Profile 1",
		"--cookie-db", "/tmp/Cookies",
		"--forget-cookie-source",
		"two.png",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if !opts.repoSet || opts.repoFlag != "owner/repo" {
		t.Fatalf("unexpected repo opts: %#v", opts)
	}
	if got := opts.cookieOpts.Browser; got != "chrome" {
		t.Fatalf("Browser = %q, want chrome", got)
	}
	if got := opts.cookieOpts.Profile; got != "Profile 1" {
		t.Fatalf("Profile = %q, want Profile 1", got)
	}
	if got := opts.cookieOpts.CookieDB; got != "/tmp/Cookies" {
		t.Fatalf("CookieDB = %q, want /tmp/Cookies", got)
	}
	if !opts.cookieOpts.ForgetRememberedSource {
		t.Fatalf("ForgetRememberedSource = false, want true")
	}
	if len(opts.imagePaths) != 2 || opts.imagePaths[0] != "one.png" || opts.imagePaths[1] != "two.png" {
		t.Fatalf("unexpected image paths: %#v", opts.imagePaths)
	}
}

func TestParseArgsRejectsDuplicateBrowser(t *testing.T) {
	_, err := parseArgs([]string{"--browser", "chrome", "--browser=brave", "one.png"})
	if err == nil || err.Error() != "--browser specified more than once" {
		t.Fatalf("err = %v, want duplicate browser error", err)
	}
}

func TestParseArgsRequiresImagePath(t *testing.T) {
	_, err := parseArgs([]string{"--browser", "chrome"})
	if err == nil || err.Error() != usage {
		t.Fatalf("err = %v, want usage error", err)
	}
}
