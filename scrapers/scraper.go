// Package scrapers contains one scraper per gacha game.
package scrapers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Character is one scraped character. Aliases hold names in other languages.
type Character struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

// Scraper scrapes a single gacha game.
type Scraper interface {
	// Name must match the entry in config.json and is used as the data file name.
	Name() string
	// Scrape receives the previously saved characters (may be empty) and returns
	// the full updated list. New characters are placed first.
	Scrape(existing []Character) ([]Character, error)
}

// LoadData reads data/<name>.json. A missing file yields an empty list.
func LoadData(dir, name string) ([]Character, error) {
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var chars []Character
	if err := json.Unmarshal(b, &chars); err != nil {
		return nil, fmt.Errorf("parse %s.json: %w", name, err)
	}
	return chars, nil
}

// SaveData writes data/<name>.json.
func SaveData(dir, name string, chars []Character) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(chars, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), append(b, '\n'), 0o644)
}

// userAgent mimics Firefox.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0"

var client = &http.Client{Timeout: 30 * time.Second}

// get fetches a URL politely (Firefox User-Agent).
func get(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}
