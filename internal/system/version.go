package system

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// VersionInfo is the JSON shape returned by GET /api/version.
type VersionInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
}

// CurrentVersion is the build-time version string. It is read from
// APP_VERSION at runtime, falling back to "0.0.0" when unset.
var CurrentVersion = func() string {
	if v := os.Getenv("APP_VERSION"); v != "" {
		return v
	}
	return "0.0.0"
}()

// VersionFetcher is the function the handler uses to fetch the latest
// version from the npm registry. It returns the version string, or
// ("", nil) on any error / timeout.
type VersionFetcher func(ctx context.Context) (string, error)

// defaultVersionFetcher queries the npm registry with a 4-second
// timeout. Errors are swallowed — the handler always returns 200.
func defaultVersionFetcher(ctx context.Context) (string, error) {
	packageName := os.Getenv("NPM_PACKAGE_NAME")
	if packageName == "" {
		packageName = "9router"
	}
	url := "https://registry.npmjs.org/" + packageName + "/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return payload.Version, nil
}

// VersionHandler returns an http.HandlerFunc for GET /api/version. It
// always returns 200 — npm fetch failures degrade gracefully to
// latestVersion="" and hasUpdate=false.
func VersionHandler() http.HandlerFunc {
	return VersionHandlerWith(defaultVersionFetcher, CurrentVersion)
}

// VersionHandlerWith is the testable form of VersionHandler.
func VersionHandlerWith(fetcher VersionFetcher, current string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		latest, _ := fetcher(ctx)
		info := VersionInfo{
			CurrentVersion: current,
			LatestVersion:  latest,
			HasUpdate:      latest != "" && compareVersions(latest, current) > 0,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	}
}

// compareVersions returns 1 if a > b, -1 if a < b, 0 if equal.
func compareVersions(a, b string) int {
	pa := splitVersion(a)
	pb := splitVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func splitVersion(v string) [3]int {
	out := [3]int{}
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}
