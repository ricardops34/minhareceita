package download

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

const (
	ckanPkgPath           = "/ckan/api/3/action/package_show?id="
	nationalTreasureBase  = "https://www.tesourotransparente.gov.br"
	nationalTreasurePkgID = "abb968cb-3710-4f85-89cf-875c91b9c7f6"
)

type ckanResource struct {
	URL string `json:"url"`
}

type ckanResult struct {
	Resources []ckanResource `json:"resources"`
}

type ckanPkg struct {
	Success bool       `json:"success"`
	Result  ckanResult `json:"result"`
}

func ckanURLs(baseURL, pkgID string) ([]string, error) {
	u := baseURL + ckanPkgPath + pkgID
	r, err := http.Get(u)
	if err != nil {
		return nil, fmt.Errorf("error getting %s: %w", u, err)
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			slog.Warn("could not close http response", "url", u, "error", err)
		}
	}()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s responded with %s", u, r.Status)
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response from %s: %w", u, err)
	}
	var pkg ckanPkg
	if err = json.Unmarshal(b, &pkg); err != nil {
		return nil, fmt.Errorf("error unmarshalling response from %s: %w", u, err)
	}
	if !pkg.Success {
		return nil, fmt.Errorf("error in ckan api response:\n%s", string(b))
	}
	urls := make([]string, len(pkg.Result.Resources))
	for i, s := range pkg.Result.Resources {
		urls[i] = s.URL
	}
	return urls, nil
}

func downloadURL(url, pth string) (int64, error) {
	f, err := os.Create(pth)
	if err != nil {
		return 0, fmt.Errorf("could not create %s: %w", pth, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close file", "path", pth, "error", err)
		}
	}()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("error creating request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error requesting %s: %w", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("could not close http response", "url", url, "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s responded with %s", url, resp.Status)
	}
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return n, fmt.Errorf("error writing to %s: %w", pth, err)
	}
	return n, nil
}
