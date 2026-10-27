package geoip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Update checks for a new release and installs it (synchronously; the job and the first download use it).
// ErrDisabled while geoip_enable is off, ErrBusy while another update runs. A failed or invalid download keeps
// the current database; the error is returned and shown in Status until the next successful check.
func (s *Service) Update(ctx context.Context) (Outcome, error) {
	if !s.Enabled() {
		return Outcome{}, ErrDisabled
	}
	if !s.busy.CompareAndSwap(false, true) {
		return Outcome{}, ErrBusy
	}
	defer s.busy.Store(false)
	out := s.run(ctx)
	return out, out.Err
}

// Begin starts an update in the background (POST /admin/geoip/update) and returns a channel that receives its
// outcome once. Same errors as Update, plus ErrStopped after the App closed.
func (s *Service) Begin() (<-chan Outcome, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if !s.busy.CompareAndSwap(false, true) {
		return nil, ErrBusy
	}
	ch := make(chan Outcome, 1)
	if !s.background(func() {
		// The flag is cleared before the outcome is sent: the receiver answers with Status(), which must not
		// still say updating.
		out := func() Outcome {
			defer s.busy.Store(false)
			return s.run(s.ctx)
		}()
		ch <- out
	}) {
		s.busy.Store(false)
		return nil, ErrStopped
	}
	return ch, nil
}

// run performs one update and records its result (the caller holds the busy flag).
func (s *Service) run(ctx context.Context) Outcome {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	s.ensureLoaded()
	updated, err := s.fetch(ctx)
	now := time.Now().UTC()
	s.mu.Lock()
	if err != nil {
		s.lastErr, s.lastErrAt = err.Error(), now
	} else {
		s.lastErr, s.lastErrAt = "", time.Time{}
		s.meta.CheckedAt = now
	}
	m := s.meta
	s.mu.Unlock()
	if err != nil {
		s.a.Log.Warn("geoip: update failed, keeping the current database", "err", err, "version", m.Version)
		return Outcome{Version: m.Version, Err: err}
	}
	if m.SHA256 != "" {
		if werr := s.writeMeta(m); werr != nil {
			s.a.Log.Warn("geoip: cannot write "+metaName, "err", werr)
		}
	}
	if updated {
		s.a.Log.Info("geoip: database updated", "version", m.Version, "records", m.Records, "size", m.Size)
	} else {
		s.a.Log.Info("geoip: database is up to date", "version", m.Version)
	}
	return Outcome{Updated: updated, Version: m.Version}
}

var versionRe = regexp.MustCompile(`^\d{8}$`)

// fetch compares the published version with the local one and downloads, validates and installs a new file.
func (s *Service) fetch(ctx context.Context) (bool, error) {
	dlURL, verURL := s.a.Settings.String(KeyDownloadURL), s.a.Settings.String(KeyVersionURL)
	if dlURL == "" {
		return false, errors.New("no download URL (" + KeyDownloadURL + ") is set")
	}
	cur := s.db.Load()
	s.mu.Lock()
	local := s.meta
	s.mu.Unlock()

	latest := ""
	if verURL != "" {
		v, err := s.latestVersion(ctx, verURL)
		if err != nil {
			return false, fmt.Errorf("version check: %w", err)
		}
		latest = v
		if cur != nil && local.Version == latest {
			return false, nil
		}
	}

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return false, err
	}
	f, err := os.CreateTemp(s.dir, datName+".*.tmp")
	if err != nil {
		return false, err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op after the rename
	h := sha256.New()
	size, err := s.download(ctx, dlURL, f, h)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return false, fmt.Errorf("download: %w", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if cur != nil && sum == local.SHA256 {
		return false, nil // same file (the mirror has no newer release yet, or there is no version URL)
	}

	data, err := os.ReadFile(tmp) //nolint:gosec // G304: the temporary file this function created in the data dir
	if err != nil {
		return false, err
	}
	db, err := validateDownload(data)
	if err != nil {
		return false, fmt.Errorf("downloaded file rejected: %w", err)
	}
	version := db.Version()
	if version == "" {
		version = latest
	}
	if latest != "" && version != latest {
		s.a.Log.Warn("geoip: the downloaded file is not the published version", "file", version, "published", latest)
	}
	if err := os.Rename(tmp, s.datPath()); err != nil {
		return false, err
	}
	syncDir(s.dir)
	s.install(db, Meta{
		Version: version, Size: size, SHA256: sum, Records: db.Count(), FetchedAt: time.Now().UTC(), Source: dlURL,
	})
	return true, nil
}

// validateDownload parses data and checks that it looks like a full release.
func validateDownload(data []byte) (*DB, error) {
	db, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if db.Count() < minRecords {
		return nil, fmt.Errorf("%d ranges, expected at least %d", db.Count(), minRecords)
	}
	if db.Locate(probeIP) == "" {
		return nil, fmt.Errorf("%s has no location", probeIP)
	}
	return db, nil
}

// latestVersion reads {"latest": "YYYYMMDD"} from version.json.
func (s *Service) latestVersion(ctx context.Context, url string) (string, error) {
	body, err := s.get(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	var v struct {
		Latest string `json:"latest"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxVersionFile)).Decode(&v); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if !versionRe.MatchString(v.Latest) {
		return "", fmt.Errorf("unexpected latest version %q", v.Latest)
	}
	return v.Latest, nil
}

var errTooLarge = errors.New("file too large")

// download streams url into w (and h), at most maxDownload bytes.
func (s *Service) download(ctx context.Context, url string, w io.Writer, h hash.Hash) (int64, error) {
	body, err := s.get(ctx, url)
	if err != nil {
		return 0, err
	}
	defer func() { _ = body.Close() }()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(body, maxDownload+1))
	if err != nil {
		return n, err
	}
	if n > maxDownload {
		return n, fmt.Errorf("%w (more than %d bytes)", errTooLarge, maxDownload)
	}
	return n, nil
}

// get performs a GET and returns the body of a 200 response.
func (s *Service) get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ggboard/"+s.a.Version+" (geoip)")
	resp, err := s.client.Do(req) //nolint:gosec // the URL is an admin setting (http/https only)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %d", filepath.Base(req.URL.Path), resp.StatusCode)
	}
	if resp.ContentLength > maxDownload {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w (%d bytes)", errTooLarge, resp.ContentLength)
	}
	return resp.Body, nil
}
