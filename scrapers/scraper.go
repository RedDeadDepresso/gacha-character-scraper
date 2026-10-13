// Package scrapers contains one scraper per gacha game.
package scrapers

import (
	"bytes"
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
	// Name must match the entry in config/games.json and is used as the data file name.
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
	b, err := marshalJSON(chars)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), b, 0o644)
}

// marshalJSON indents like json.MarshalIndent but does not escape &, < and >
// as \u0026, \u003c and \u003e.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil { // Encode ends with a newline
		return nil, err
	}
	return buf.Bytes(), nil
}

// userAgent mimics Firefox.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0"

// ErrNotFound is returned (wrapped) by get when the server answers 404.
var ErrNotFound = errors.New("not found")

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
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %w", url, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// ConfigDir holds games.json and the commit cache.
const ConfigDir = "config"

const commitsFile = "commits.json"

// LoadCommit returns the last scraped commit SHA for a game ("" if none).
func LoadCommit(name string) string {
	b, err := os.ReadFile(filepath.Join(ConfigDir, commitsFile))
	if err != nil {
		return ""
	}
	m := map[string]string{}
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m[name]
}

// SaveCommit stores the commit SHA for a game in config/commits.json.
func SaveCommit(name, sha string) error {
	path := filepath.Join(ConfigDir, commitsFile)
	m := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	m[name] = sha
	if err := os.MkdirAll(ConfigDir, 0o755); err != nil {
		return err
	}
	b, err := marshalJSON(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// LatestCommit returns the SHA of the newest commit on a GitHub branch.
// Set GITHUB_TOKEN to raise the API rate limit (optional).
func LatestCommit(repo, branch string) (string, error) {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, branch), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api %s: %s", repo, resp.Status)
	}
	var out struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.SHA == "" {
		return "", fmt.Errorf("github api %s: empty sha", repo)
	}
	return out.SHA, nil
}
