# gh-image

![banner](https://github.com/user-attachments/assets/92463e67-b897-4212-91b4-a4f9b80ec4d4)

A `gh` CLI extension that uploads images to GitHub from the command line.

GitHub has no public API for image uploads — the web UI uses an internal endpoint that produces URLs scoped to a repository's visibility. This tool replicates that flow, so images on private repos stay private.

## Usage

```bash
# Upload an image and get the markdown (infers repo from current git workspace)
gh image screenshot.png

# Upload multiple images
gh image img1.png img2.png

# Explicit repo (when not in a git workspace or targeting a different repo)
gh image screenshot.png --repo owner/repo

# Restrict cookie lookup to a known browser/profile
gh image screenshot.png --browser chrome --profile "Profile 1"

# Read from an explicit Chromium cookie DB
gh image screenshot.png --browser chrome --cookie-db "$HOME/Library/Application Support/Google/Chrome/Profile 1/Cookies"
```

Output:
```
![screenshot](https://github.com/user-attachments/assets/<uuid>)
```

### Example: Upload an image and attach it to a GitHub issue

```bash
# Upload the image
gh image screenshot.png --repo owner/repo
# Output: ![screenshot.png](https://github.com/user-attachments/assets/88f4599a-...)

# Use the output in a new issue
gh issue create --repo owner/repo \
  --title "Bug report" \
  --body "Here's what I see:

![screenshot.png](https://github.com/user-attachments/assets/88f4599a-...)
"
```

## Install

```bash
gh extension install drogers0/gh-image
```

## How It Works

1. Reads your GitHub session cookie from your browser's local cookie store (encrypted cookie decryption handled automatically)
2. Uploads the image through GitHub's internal asset upload flow (the same one the web UI uses)
3. Prints a markdown image reference to stdout

The upload produces `https://github.com/user-attachments/assets/<uuid>` URLs — the same format as drag-and-drop uploads in the browser. Images inherit the repository's visibility: private repo images require authentication to view.

See [documentation/github-image-upload-flow.md](documentation/github-image-upload-flow.md) for the full reverse-engineered upload protocol.

## Authentication

No tokens or OAuth setup required. The tool reads your `user_session` cookie directly from your browser's cookie database on disk.

To reduce repeated prompts, `gh-image` remembers the last successful cookie source (browser, profile, and cookie DB path) in your user config directory and tries that source first on the next run before scanning all supported browsers.

On macOS, a Keychain prompt may still appear when the selected browser cookie DB needs access to the browser's Safe Storage key. `gh-image` does not persist or bypass that browser decryption credential; it only remembers which browser/profile/path to try first.

Useful flags:
- `--browser chrome|brave|edge|chromium` to restrict lookup to one browser
- `--profile "Profile 1"` to restrict lookup to one profile
- `--cookie-db /absolute/path/to/Cookies` to use a specific Chromium cookie DB
- `--forget-cookie-source` to clear the remembered source before lookup

Supported browsers:
- Chrome
- Brave
- Chromium
- Edge

Supported platforms:
- macOS
- Linux
- Windows

## Requirements

- A supported browser with an active GitHub session
- Write access to the target repository

## Limitations

- This tool uses an undocumented GitHub internal API that could change without notice.
- The `uploadToken` required for uploads is only available to users with write access to the target repository.
- Either `--repo` must be provided or the tool must be run from within a git workspace with a GitHub remote.
- Remembering the cookie source reduces scanning and repeated prompts, but it does not make macOS Keychain authorization persist across runs unless you change Keychain access policy yourself.
