package geoip

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
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
//
// The errors of a download, a version check and a request end up as last_error in GET /admin/geoip (and in the
// log). The URLs are admin settings that may point at internal services (allowed, MISTAKES #53), so these errors
// say what was wrong without repeating what the other side sent: no field values, offsets or lengths read from
// a body, no quoted status line, no redirect target.
func validateDownload(data []byte) (*DB, error) {
	db, err := Parse(data)
	if err != nil {
		return nil, ErrFormat // Parse's detail (index bounds, length) is read from the body
	}
	if db.Count() < minRecords {
		return nil, fmt.Errorf("fewer than %d IP ranges", minRecords)
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
		return "", errors.New(`the version file is not JSON like {"latest": "YYYYMMDD"}`)
	}
	if !versionRe.MatchString(v.Latest) {
		return "", errors.New(`"latest" in the version file is not a YYYYMMDD date`)
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
	name := filepath.Base(req.URL.Path)
	resp, err := s.client.Do(req) //nolint:gosec // the URL is an admin setting (http/https only)
	if err != nil {
		return nil, &requestError{msg: "GET " + name + ": " + requestFailure(err), err: err}
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %d", name, resp.StatusCode)
	}
	if resp.ContentLength > maxDownload {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w (more than %d bytes)", errTooLarge, maxDownload)
	}
	return resp.Body, nil
}

// maxRedirects is how many redirects one GET follows (Go's default; the release URL takes two).
const maxRedirects = 10

var (
	errTooManyRedirects = errors.New("too many redirects")
	errInsecureRedirect = errors.New("redirect from https to plain http refused")
)

// checkRedirect is the update client's CheckRedirect. Redirects to other hosts are followed: the default download
// URL moves from github.com to release-assets.githubusercontent.com. A step down from https to plain http is not.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errInsecureRedirect
	}
	return nil
}

// requestError is a failed request: Error() is the redacted message, Unwrap the original error (for errors.Is).
type requestError struct {
	msg string
	err error
}

func (e *requestError) Error() string { return e.msg }
func (e *requestError) Unwrap() error { return e.err }

// requestFailure says why client.Do failed without repeating the other side's words: the *url.Error names the
// last URL (a redirect target) and Go's transport quotes a malformed status line (`malformed HTTP response
// "SSH-2.0-…"`, a banner grab).
func requestFailure(err error) string {
	var (
		dnsErr  *net.DNSError
		netErr  net.Error
		certErr *tls.CertificateVerificationError
		hdrErr  tls.RecordHeaderError
	)
	switch {
	case errors.Is(err, errTooManyRedirects):
		return errTooManyRedirects.Error()
	case errors.Is(err, errInsecureRedirect):
		return errInsecureRedirect.Error()
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "timed out"
	case errors.As(err, &dnsErr):
		return "host not found"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE), errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF):
		return "the connection closed without a response"
	case errors.As(err, &certErr):
		return "TLS certificate not trusted"
	case errors.As(err, &hdrErr):
		return "the server did not answer with TLS"
	// A banner that arrives before the request is written makes Go's transport give up with readLoopPeekFailLocked
	// (an HTTP server never speaks first); one that arrives after it is a malformed status line.
	case strings.Contains(err.Error(), "malformed HTTP"), strings.Contains(err.Error(), "readLoopPeekFailLocked"):
		return "the server did not answer with HTTP"
	}
	return "request failed"
}
