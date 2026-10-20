// Package bangumi reads the Bangumi Archive (https://github.com/bangumi/Archive)
// to give gacha characters a popularity score and to spot collab characters.
//
// Update downloads the newest dump the same way agv-character-scraper does: only
// the three files that are needed are extracted, the zip is deleted afterwards.
// Load then reads them (see load.go) and Data.Match (see match.go) links the
// scraped characters of a game to the Bangumi characters of its subject.
package bangumi

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// The only files of the dump that are needed (the rest is skipped).
const (
	SubjectsFile   = "subject.jsonlines"
	LinksFile      = "subject-characters.jsonlines"
	CharactersFile = "character.jsonlines"

	// latestFile remembers which release the files in the dump directory come from.
	latestFile = "latest.json"
)

// Needed lists the files extracted from the dump.
var Needed = []string{SubjectsFile, LinksFile, CharactersFile}

// latestURL is the Archive's aux/latest.json (the release asset of the newest dump).
// It is a variable so tests can point it at a local server.
var latestURL = "https://raw.githubusercontent.com/bangumi/Archive/refs/heads/master/aux/latest.json"

const userAgent = "gacha-character-scraper"

// The dump is several hundred MB, so only the connection set-up is bounded.
var httpClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	TLSHandshakeTimeout:   30 * time.Second,
	ResponseHeaderTimeout: 2 * time.Minute,
}}

// Release is the part of aux/latest.json that is used. The whole file is saved
// in the dump directory and compared with the remote one on the next run.
type Release struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Digest      string `json:"digest"` // "sha256:<hex>"
	Size        int64  `json:"size"`
}

func get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

func fetchLatest() (Release, []byte, error) {
	resp, err := get(latestURL)
	if err != nil {
		return Release{}, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Release{}, nil, err
	}
	var rel Release
	if err := json.Unmarshal(raw, &rel); err != nil {
		return Release{}, nil, fmt.Errorf("parse %s: %w", latestURL, err)
	}
	if rel.Name == "" || rel.DownloadURL == "" {
		return Release{}, nil, fmt.Errorf("%s has no name / browser_download_url", latestURL)
	}
	return rel, raw, nil
}

// complete reports whether dir holds latest.json and every needed file.
func complete(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, latestFile)); err != nil {
		return false
	}
	for _, n := range Needed {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			return false
		}
	}
	return true
}

// Update makes dir hold the newest dump and downloads nothing when latest.json
// is unchanged. If latest.json cannot be fetched, the files already in dir are
// used (an error is returned only when there are none).
func Update(dir string) error {
	remote, raw, err := fetchLatest()
	if err != nil {
		if complete(dir) {
			log.Printf("bangumi: could not fetch latest.json (%v), using the dump already in %s", err, dir)
			return nil
		}
		return fmt.Errorf("could not fetch %s: %w", latestURL, err)
	}

	if b, err := os.ReadFile(filepath.Join(dir, latestFile)); err == nil && complete(dir) {
		var local Release
		if json.Unmarshal(b, &local) == nil && local == remote {
			log.Printf("bangumi: dump is up to date (%s), skipping the download", remote.Name)
			return nil
		}
	}

	log.Printf("bangumi: downloading %s (%.0f MB)...", remote.Name, float64(remote.Size)/1e6)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// dir is inconsistent until we finish.
	if err := os.Remove(filepath.Join(dir, latestFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The name comes from the network: never let it leave dir.
	archive := filepath.Join(dir, filepath.Base(remote.Name))
	defer os.Remove(archive)
	if err := download(remote.DownloadURL, archive, remote.Digest); err != nil {
		return err
	}
	log.Printf("bangumi: extracting %s...", strings.Join(Needed, ", "))
	if err := extract(archive, dir, Needed); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, latestFile), raw, 0o644)
}

// download streams url to dest and checks digest ("sha256:<hex>") when given.
func download(url, dest, digest string) error {
	resp, err := get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	sum := sha256.New()
	buf := make([]byte, 1<<20)
	var done int64
	last := int64(-1)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			sum.Write(buf[:n])
			done += int64(n)
			if resp.ContentLength > 0 && done*20/resp.ContentLength != last {
				last = done * 20 / resp.ContentLength
				log.Printf("bangumi:   downloaded %.0f / %.0f MB", float64(done)/1e6, float64(resp.ContentLength)/1e6)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if algo, want, ok := strings.Cut(digest, ":"); ok && algo == "sha256" {
		if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, want) {
			return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", filepath.Base(dest), want, got)
		}
	}
	return nil
}

// extract copies only names out of the zip (matched by file name, wherever the
// file sits in the archive). A file is written as <name>.part and renamed, so a
// truncated file never has its real name.
func extract(archive, dir string, names []string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()

	members := map[string]*zip.File{}
	for _, f := range z.File {
		if !f.FileInfo().IsDir() {
			members[path.Base(f.Name)] = f
		}
	}
	var missing []string
	for _, n := range names {
		if members[n] == nil {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s does not contain %s", filepath.Base(archive), strings.Join(missing, ", "))
	}
	for _, n := range names {
		if err := extractOne(members[n], filepath.Join(dir, n)); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, dest string) (err error) {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	part := dest + ".part"
	dst, err := os.Create(part)
	if err != nil {
		return err
	}
	defer func() {
		dst.Close()
		os.Remove(part) // no-op after a successful rename
	}()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Rename(part, dest)
}
