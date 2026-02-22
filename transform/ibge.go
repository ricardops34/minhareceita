package transform

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

const (
	ibgeMunicipalitiesCKANURL = "https://www.tesourotransparente.gov.br/ckan/api/3/action/package_show?id=abb968cb-3710-4f85-89cf-875c91b9c7f6"
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

// ibgeMunicipalitiesURL returns the download URL for the tabmun CSV from the National
// Treasure (Tesouro Nacional) open data portal.
func ibgeMunicipalitiesURL() (string, error) {
	resp, err := http.Get(ibgeMunicipalitiesCKANURL)
	if err != nil {
		return "", fmt.Errorf("error fetching tabmun package metadata: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("could not close tabmun metadata response", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tabmun metadata request returned %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading tabmun package metadata: %w", err)
	}
	var pkg ckanPkg
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "", fmt.Errorf("error parsing tabmun package metadata: %w", err)
	}
	if !pkg.Success || len(pkg.Result.Resources) == 0 {
		return "", fmt.Errorf("no resources found in tabmun package metadata")
	}
	return pkg.Result.Resources[0].URL, nil
}
