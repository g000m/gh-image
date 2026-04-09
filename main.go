package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/drogers0/gh-image/internal/cookies"
	"github.com/drogers0/gh-image/internal/repo"
	"github.com/drogers0/gh-image/internal/upload"
)

const usage = "Usage: gh image [--repo owner/repo] [--browser name] [--profile name] [--cookie-db path] <image-path>..."

type cliOptions struct {
	repoFlag   string
	repoSet    bool
	imagePaths []string
	cookieOpts cookies.Options
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		if err == errHelp {
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'gh image --help' for usage.\n")
		os.Exit(1)
	}

	// Validate image paths early
	for _, p := range opts.imagePaths {
		if p == "" {
			fmt.Fprintf(os.Stderr, "Error: empty image path\n")
			os.Exit(1)
		}
	}

	// Resolve repository
	var owner, name string
	if opts.repoSet {
		if opts.repoFlag == "" {
			fmt.Fprintf(os.Stderr, "Error: --repo value cannot be empty\n")
			os.Exit(1)
		}
		parts := strings.SplitN(opts.repoFlag, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			fmt.Fprintf(os.Stderr, "Error: --repo must be in owner/repo format, got: %s\n", opts.repoFlag)
			os.Exit(1)
		}
		owner, name = parts[0], parts[1]
	}

	repoInfo, err := repo.Resolve(owner, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving repository: %v\n", err)
		os.Exit(1)
	}

	cookie, err := cookies.GetGitHubSession(opts.cookieOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client := upload.NewClient(cookie)

	hasError := false
	for _, imagePath := range opts.imagePaths {
		result, err := upload.Upload(client, repoInfo.Owner, repoInfo.Name, repoInfo.ID, imagePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error uploading %s: %v\n", imagePath, err)
			hasError = true
			continue
		}
		fmt.Println(result.Markdown)
	}
	if hasError {
		os.Exit(1)
	}
}

var errHelp = fmt.Errorf("help requested")

func parseArgs(args []string) (cliOptions, error) {
	var opts cliOptions
	flagsDone := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if flagsDone {
			opts.imagePaths = append(opts.imagePaths, arg)
			continue
		}

		switch {
		case arg == "--":
			flagsDone = true
		case arg == "--repo":
			value, next, err := requireValue(args, i, "--repo")
			if err != nil {
				return cliOptions{}, err
			}
			if opts.repoSet {
				return cliOptions{}, fmt.Errorf("--repo specified more than once")
			}
			opts.repoFlag = value
			opts.repoSet = true
			i = next
		case strings.HasPrefix(arg, "--repo="):
			if opts.repoSet {
				return cliOptions{}, fmt.Errorf("--repo specified more than once")
			}
			opts.repoFlag = strings.SplitN(arg, "=", 2)[1]
			opts.repoSet = true
		case arg == "--browser":
			value, next, err := requireValue(args, i, "--browser")
			if err != nil {
				return cliOptions{}, err
			}
			if opts.cookieOpts.Browser != "" {
				return cliOptions{}, fmt.Errorf("--browser specified more than once")
			}
			opts.cookieOpts.Browser = value
			i = next
		case strings.HasPrefix(arg, "--browser="):
			if opts.cookieOpts.Browser != "" {
				return cliOptions{}, fmt.Errorf("--browser specified more than once")
			}
			opts.cookieOpts.Browser = strings.SplitN(arg, "=", 2)[1]
		case arg == "--profile":
			value, next, err := requireValue(args, i, "--profile")
			if err != nil {
				return cliOptions{}, err
			}
			if opts.cookieOpts.Profile != "" {
				return cliOptions{}, fmt.Errorf("--profile specified more than once")
			}
			opts.cookieOpts.Profile = value
			i = next
		case strings.HasPrefix(arg, "--profile="):
			if opts.cookieOpts.Profile != "" {
				return cliOptions{}, fmt.Errorf("--profile specified more than once")
			}
			opts.cookieOpts.Profile = strings.SplitN(arg, "=", 2)[1]
		case arg == "--cookie-db":
			value, next, err := requireValue(args, i, "--cookie-db")
			if err != nil {
				return cliOptions{}, err
			}
			if opts.cookieOpts.CookieDB != "" {
				return cliOptions{}, fmt.Errorf("--cookie-db specified more than once")
			}
			opts.cookieOpts.CookieDB = value
			i = next
		case strings.HasPrefix(arg, "--cookie-db="):
			if opts.cookieOpts.CookieDB != "" {
				return cliOptions{}, fmt.Errorf("--cookie-db specified more than once")
			}
			opts.cookieOpts.CookieDB = strings.SplitN(arg, "=", 2)[1]
		case arg == "--forget-cookie-source":
			opts.cookieOpts.ForgetRememberedSource = true
		case arg == "--help" || arg == "-h":
			printHelp()
			return cliOptions{}, errHelp
		case strings.HasPrefix(arg, "-") && arg != "-":
			return cliOptions{}, fmt.Errorf("unknown flag %s", arg)
		default:
			opts.imagePaths = append(opts.imagePaths, arg)
		}
	}

	if len(opts.imagePaths) == 0 {
		return cliOptions{}, fmt.Errorf(usage)
	}

	return opts, nil
}

func requireValue(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("%s requires a value", flag)
	}
	return args[i+1], i + 1, nil
}

func printHelp() {
	fmt.Printf("%s\n\n", usage)
	fmt.Println("Upload images to GitHub and print markdown references.")
	fmt.Println()
	fmt.Println("The --repo flag is optional. If omitted, the repository is")
	fmt.Println("inferred from the git remote in the current directory.")
	fmt.Println()
	fmt.Println("Cookie lookup remembers the last successful browser/profile/path")
	fmt.Println("and tries it first on the next run.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --repo owner/repo         GitHub repository (optional)")
	fmt.Println("  --browser name            Restrict cookie lookup to chrome, brave, edge, or chromium")
	fmt.Println("  --profile name            Restrict cookie lookup to a browser profile")
	fmt.Println("  --cookie-db path          Read cookies from an explicit Chromium cookie DB path")
	fmt.Println("  --forget-cookie-source    Clear the remembered browser/profile/path before lookup")
	fmt.Println()
	fmt.Println("Use -- to separate flags from filenames starting with a dash:")
	fmt.Println("  gh image -- -screenshot.png")
}
