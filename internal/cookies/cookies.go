package cookies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/browserutils/kooky"
	bravebrowser "github.com/browserutils/kooky/browser/brave"
	chromebrowser "github.com/browserutils/kooky/browser/chrome"
	chromiumbrowser "github.com/browserutils/kooky/browser/chromium"
	edgebrowser "github.com/browserutils/kooky/browser/edge"
)

const (
	configDirName  = "gh-image"
	configFileName = "config.json"
)

var (
	userConfigDir = os.UserConfigDir

	supportedBrowsers = map[string]int{
		"chrome":   0,
		"brave":    1,
		"edge":     2,
		"chromium": 3,
	}
)

type Options struct {
	Browser                string
	Profile                string
	CookieDB               string
	ForgetRememberedSource bool
}

type configFile struct {
	CookieSource *Source `json:"cookie_source,omitempty"`
}

type Source struct {
	Browser  string `json:"browser,omitempty"`
	Profile  string `json:"profile,omitempty"`
	CookieDB string `json:"cookie_db,omitempty"`
}

type candidateStore struct {
	store  kooky.CookieStore
	source Source
}

// GetGitHubSession returns the user_session cookie for github.com.
// If a source was remembered from a previous run, it is tried first.
// Explicit browser/profile/path options restrict lookup to matching sources.
func GetGitHubSession(opts Options) (*http.Cookie, error) {
	opts = normalizeOptions(opts)
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	if opts.ForgetRememberedSource {
		if err := clearRememberedSource(); err != nil {
			return nil, err
		}
	}

	ctx := context.Background()
	filters := []kooky.Filter{
		kooky.Valid,
		kooky.DomainHasSuffix("github.com"),
		kooky.Name("user_session"),
	}

	if opts.CookieDB != "" {
		source := Source{
			Browser:  opts.Browser,
			Profile:  opts.Profile,
			CookieDB: opts.CookieDB,
		}
		if source.Browser == "" {
			source.Browser = inferBrowserFromPath(source.CookieDB)
			if source.Browser == "" {
				return nil, fmt.Errorf("cannot infer browser from --cookie-db path %q; pass --browser", opts.CookieDB)
			}
		}
		cookie, resolvedSource, err := readFromDirectSource(ctx, source, filters)
		if err != nil {
			return nil, err
		}
		_ = saveRememberedSource(resolvedSource)
		return cookie, nil
	}

	stores, discoveryErr := discoverStores(ctx)
	defer closeStores(stores)

	if len(stores) == 0 {
		if discoveryErr != nil {
			return nil, fmt.Errorf("reading browser cookies: %w", discoveryErr)
		}
		return nil, fmt.Errorf("no supported browser cookie stores found")
	}

	if opts.Browser != "" || opts.Profile != "" {
		matches := filterStores(stores, opts)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no supported browser cookie store matched %s", describeLookup(opts))
		}
		cookie, source, err := readFromStores(ctx, matches, filters)
		if err != nil {
			return nil, err
		}
		_ = saveRememberedSource(source)
		return cookie, nil
	}

	remembered, ok, err := loadRememberedSource()
	if err != nil {
		return nil, err
	}
	if ok {
		if matches := exactSourceMatches(stores, remembered); len(matches) > 0 {
			cookie, source, err := readFromStores(ctx, matches, filters)
			if err == nil {
				_ = saveRememberedSource(source)
				return cookie, nil
			}
		} else if remembered.CookieDB != "" {
			cookie, source, err := readFromDirectSource(ctx, remembered, filters)
			if err == nil {
				_ = saveRememberedSource(source)
				return cookie, nil
			}
		}
		stores = excludeSource(stores, remembered)
	}

	cookie, source, err := readFromStores(ctx, stores, filters)
	if err != nil {
		if discoveryErr != nil {
			return nil, fmt.Errorf("reading browser cookies: %w", errors.Join(discoveryErr, err))
		}
		return nil, err
	}
	_ = saveRememberedSource(source)
	return cookie, nil
}

func normalizeOptions(opts Options) Options {
	opts.Browser = strings.ToLower(strings.TrimSpace(opts.Browser))
	opts.Profile = strings.TrimSpace(opts.Profile)
	opts.CookieDB = strings.TrimSpace(opts.CookieDB)
	return opts
}

func validateOptions(opts Options) error {
	if opts.Browser != "" {
		if _, ok := supportedBrowsers[opts.Browser]; !ok {
			return fmt.Errorf("unsupported browser %q (supported: chrome, brave, edge, chromium)", opts.Browser)
		}
	}
	if opts.Profile == "" && opts.CookieDB == "" {
		return nil
	}
	return nil
}

func discoverStores(ctx context.Context) ([]candidateStore, error) {
	var stores []candidateStore
	var errs []error

	for store, err := range kooky.TraverseCookieStores(ctx) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if store == nil {
			continue
		}
		stores = append(stores, candidateStore{
			store:  store,
			source: sourceFromBrowserInfo(store),
		})
	}

	sort.SliceStable(stores, func(i, j int) bool {
		return compareSources(stores[i].source, stores[j].source) < 0
	})

	return stores, errors.Join(errs...)
}

func closeStores(stores []candidateStore) {
	for _, store := range stores {
		if store.store != nil {
			_ = store.store.Close()
		}
	}
}

func readFromStores(ctx context.Context, stores []candidateStore, filters []kooky.Filter) (*http.Cookie, Source, error) {
	var errs []error
	for _, candidate := range stores {
		cookie, source, err := readFromStore(ctx, candidate, filters)
		if err == nil && cookie != nil {
			return cookie, source, nil
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", describeSource(candidate.source), err))
		}
	}

	if len(errs) > 0 {
		return nil, Source{}, fmt.Errorf("reading browser cookies: %w", errors.Join(errs...))
	}
	return nil, Source{}, fmt.Errorf("no github.com user_session cookie found in any supported browser — are you logged into GitHub?")
}

func readFromStore(ctx context.Context, candidate candidateStore, filters []kooky.Filter) (*http.Cookie, Source, error) {
	cookies, err := candidate.store.TraverseCookies(filters...).ReadAllCookies(ctx)
	if len(cookies) > 0 {
		cookie := &cookies[0].Cookie
		source := sourceFromCookie(cookies[0])
		if source.CookieDB == "" {
			source = candidate.source
		}
		return cookie, source, nil
	}
	if err != nil {
		return nil, Source{}, err
	}
	return nil, Source{}, nil
}

func readFromDirectSource(ctx context.Context, source Source, filters []kooky.Filter) (*http.Cookie, Source, error) {
	var (
		cookies []*kooky.Cookie
		err     error
	)

	switch source.Browser {
	case "chrome":
		cookies, err = chromebrowser.ReadCookies(ctx, source.CookieDB, filters...)
	case "brave":
		cookies, err = bravebrowser.ReadCookies(ctx, source.CookieDB, filters...)
	case "edge":
		cookies, err = edgebrowser.ReadCookies(ctx, source.CookieDB, filters...)
	case "chromium":
		cookies, err = chromiumbrowser.ReadCookies(ctx, source.CookieDB, filters...)
	default:
		return nil, Source{}, fmt.Errorf("unsupported browser %q", source.Browser)
	}

	if len(cookies) > 0 {
		resolved := sourceFromCookie(cookies[0])
		if resolved.Browser == "" {
			resolved.Browser = source.Browser
		}
		if resolved.Profile == "" {
			resolved.Profile = source.Profile
		}
		if resolved.CookieDB == "" {
			resolved.CookieDB = source.CookieDB
		}
		return &cookies[0].Cookie, resolved, nil
	}
	if err != nil {
		return nil, Source{}, fmt.Errorf("reading browser cookies: %w", err)
	}
	return nil, Source{}, fmt.Errorf("no github.com user_session cookie found in %s", describeSource(source))
}

func filterStores(stores []candidateStore, opts Options) []candidateStore {
	var matches []candidateStore
	for _, candidate := range stores {
		if opts.Browser != "" && !strings.EqualFold(candidate.source.Browser, opts.Browser) {
			continue
		}
		if opts.Profile != "" && !strings.EqualFold(candidate.source.Profile, opts.Profile) {
			continue
		}
		matches = append(matches, candidate)
	}
	return matches
}

func exactSourceMatches(stores []candidateStore, source Source) []candidateStore {
	var matches []candidateStore
	for _, candidate := range stores {
		if !sameSource(candidate.source, source) {
			continue
		}
		matches = append(matches, candidate)
	}
	return matches
}

func excludeSource(stores []candidateStore, source Source) []candidateStore {
	var filtered []candidateStore
	for _, candidate := range stores {
		if sameSource(candidate.source, source) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func sameSource(a, b Source) bool {
	return strings.EqualFold(a.Browser, b.Browser) &&
		strings.EqualFold(a.Profile, b.Profile) &&
		a.CookieDB == b.CookieDB
}

func compareSources(a, b Source) int {
	ai, aok := supportedBrowsers[a.Browser]
	bi, bok := supportedBrowsers[b.Browser]
	switch {
	case aok && bok && ai != bi:
		return ai - bi
	case aok && !bok:
		return -1
	case !aok && bok:
		return 1
	}
	if a.Profile == "Default" && b.Profile != "Default" {
		return -1
	}
	if a.Profile != "Default" && b.Profile == "Default" {
		return 1
	}
	if cmp := strings.Compare(strings.ToLower(a.Profile), strings.ToLower(b.Profile)); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.CookieDB, b.CookieDB)
}

func sourceFromCookie(cookie *kooky.Cookie) Source {
	if cookie == nil || cookie.Browser == nil {
		return Source{}
	}
	return sourceFromBrowserInfo(cookie.Browser)
}

func sourceFromBrowserInfo(info kooky.BrowserInfo) Source {
	if info == nil {
		return Source{}
	}
	return Source{
		Browser:  strings.ToLower(strings.TrimSpace(info.Browser())),
		Profile:  strings.TrimSpace(info.Profile()),
		CookieDB: info.FilePath(),
	}
}

func describeLookup(opts Options) string {
	var parts []string
	if opts.Browser != "" {
		parts = append(parts, fmt.Sprintf("browser %q", opts.Browser))
	}
	if opts.Profile != "" {
		parts = append(parts, fmt.Sprintf("profile %q", opts.Profile))
	}
	if opts.CookieDB != "" {
		parts = append(parts, fmt.Sprintf("cookie DB %q", opts.CookieDB))
	}
	if len(parts) == 0 {
		return "the current lookup options"
	}
	return strings.Join(parts, ", ")
}

func describeSource(source Source) string {
	var parts []string
	if source.Browser != "" {
		parts = append(parts, source.Browser)
	}
	if source.Profile != "" {
		parts = append(parts, fmt.Sprintf("profile %q", source.Profile))
	}
	if source.CookieDB != "" {
		parts = append(parts, source.CookieDB)
	}
	if len(parts) == 0 {
		return "unknown cookie source"
	}
	return strings.Join(parts, " ")
}

func inferBrowserFromPath(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "bravesoftware/brave-browser"), strings.Contains(lower, "brave-browser"):
		return "brave"
	case strings.Contains(lower, "google/chrome"), strings.Contains(lower, "chrome"):
		return "chrome"
	case strings.Contains(lower, "microsoft edge"), strings.Contains(lower, "microsoft/edge"), strings.Contains(lower, "microsoft-edge"):
		return "edge"
	case strings.Contains(lower, "chromium"):
		return "chromium"
	default:
		return ""
	}
}

func loadRememberedSource() (Source, bool, error) {
	path, err := configPath()
	if err != nil {
		return Source{}, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Source{}, false, nil
		}
		return Source{}, false, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Source{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.CookieSource == nil || cfg.CookieSource.CookieDB == "" {
		return Source{}, false, nil
	}
	return *cfg.CookieSource, true, nil
}

func saveRememberedSource(source Source) error {
	if source.CookieDB == "" {
		return nil
	}

	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(configFile{CookieSource: &source}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func clearRememberedSource() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}

func configPath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(dir, configDirName, configFileName), nil
}
