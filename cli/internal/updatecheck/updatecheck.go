package updatecheck

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	latestReleaseURL = "https://github.com/cordon-co/cordon-cli/releases/latest"
	installScriptURL = "https://raw.githubusercontent.com/cordon-co/cordon-cli/main/scripts/install.sh"
)

type config struct {
	SkipUpdateCheck bool   `json:"skip_update_check"`
	LastUpdateCheck string `json:"last_update_check"`
}

// MaybeRun performs a best-effort daily update check for interactive CLI usage.
// It never returns an error and should not affect command execution.
func MaybeRun(in io.Reader, out io.Writer, errOut io.Writer, currentVersion string) {
	if normalizeVersion(currentVersion) == "dev" {
		return
	}

	if !isInteractive(in, out) {
		return
	}

	cfgPath, err := configPath()
	if err != nil {
		return
	}

	cfg, raw, err := readConfig(cfgPath)
	if err != nil {
		return
	}
	if cfg.SkipUpdateCheck {
		return
	}

	now := time.Now().UTC()
	if checkedWithin24Hours(cfg.LastUpdateCheck, now) {
		return
	}

	cfg.LastUpdateCheck = now.Format(time.RFC3339)
	if err := writeConfig(cfgPath, cfg, raw); err != nil {
		return
	}

	latest, err := fetchLatestReleaseTag(http.DefaultClient)
	if err != nil {
		return
	}
	if !isDifferentVersion(currentVersion, latest) {
		return
	}

	fmt.Fprintf(out, "A new version of cordon-cli is available on github, install the update? [Y/n]: ")
	ok, err := readYesNo(in)
	if err != nil {
		return
	}
	if !ok {
		fmt.Fprintln(out, `Daily update checks can be disabled by setting "skip_update_check" to true in ~/.cordon/config.json`)
		return
	}

	if err := runInstaller(in, out, errOut); err != nil {
		fmt.Fprintf(errOut, "Failed to install update automatically: %v\n", err)
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cordon", "config.json"), nil
}

func readConfig(p string) (config, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return config{}, map[string]json.RawMessage{}, nil
		}
		return config{}, nil, err
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return config{}, nil, err
	}

	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, nil, err
	}

	return cfg, raw, nil
}

func writeConfig(p string, cfg config, raw map[string]json.RawMessage) error {
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}

	last, err := json.Marshal(cfg.LastUpdateCheck)
	if err != nil {
		return err
	}
	raw["last_update_check"] = last

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func checkedWithin24Hours(raw string, now time.Time) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	last, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return false
	}
	return now.Sub(last) < 24*time.Hour
}

func fetchLatestReleaseTag(client *http.Client) (string, error) {
	return fetchLatestReleaseTagFromURL(client, latestReleaseURL)
}

func fetchLatestReleaseTagFromURL(client *http.Client, latestURL string) (string, error) {
	httpClient := *client
	httpClient.Timeout = 2 * time.Second
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequest(http.MethodGet, latestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "cordon-cli")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if strings.TrimSpace(loc) == "" {
		return "", errors.New("missing redirect location")
	}

	u, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	tag := path.Base(strings.TrimSuffix(u.Path, "/"))
	if strings.TrimSpace(tag) == "" || tag == "latest" {
		return "", errors.New("invalid release tag")
	}
	return tag, nil
}

func isDifferentVersion(current string, latest string) bool {
	c := normalizeVersion(current)
	l := normalizeVersion(latest)
	if c == "" || c == "dev" || l == "" {
		return false
	}
	return c != l
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "v")
	return v
}

func readYesNo(in io.Reader) (bool, error) {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	switch answer {
	case "", "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, nil
	}
}

func runInstaller(in io.Reader, out io.Writer, errOut io.Writer) error {
	cmd := exec.Command("sh", "-c", "curl -fsSL "+installScriptURL+" | sh")
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

func isInteractive(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK {
		return false
	}
	inInfo, err := inFile.Stat()
	if err != nil {
		return false
	}
	outInfo, err := outFile.Stat()
	if err != nil {
		return false
	}
	return (inInfo.Mode()&os.ModeCharDevice) != 0 && (outInfo.Mode()&os.ModeCharDevice) != 0
}
