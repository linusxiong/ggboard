package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Scenario groups (the SVG adds the Chinese heading).
const (
	groupSub    = "Subscription"
	groupNode   = "Node API"
	groupLogin  = "Login"
	groupStatic = "Static"
)

// ruleTemplate is the catalog template the mihomo scenario asks for (14 rule categories).
const ruleTemplate = "standard"

// env is what the scenario builders need.
type env struct {
	base      string
	ds        *dataset
	admin     *api
	load      *http.Client // the scenarios' client (no transparent decompression)
	conc      int
	loginConc int
	pushBatch int
	perToken  int
	logf      func(string, ...any)
	// nodeEncoding is the Accept-Encoding of the UniProxy user-list requests ("gzip" like Go clients; "identity" to
	// measure the same path without response compression).
	nodeEncoding string

	flush      string // duration of the controlled traffic_flush window (node_push)
	flushPairs int64
}

func (e *env) get(path string, hdr map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.base+path, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return req, nil
}

// probe sends one request with transparent gzip decoding (Go's default) and returns status, headers and body.
func (e *env) probe(req *http.Request) (int, http.Header, []byte, error) {
	req.Header.Del("Accept-Encoding")
	resp, err := e.admin.hc.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b, err
}

// rawProbe sends req as is (keeps Accept-Encoding) and returns status, headers and the body length on the wire.
func (e *env) rawProbe(req *http.Request) (int, http.Header, int64, error) {
	resp, err := e.load.Do(req)
	if err != nil {
		return 0, nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	n, err := io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header, n, err
}

func (e *env) nodeQuery(n benchNode) string {
	return "?token=" + url.QueryEscape(e.ds.ServerToken) + "&node_id=" + strconv.FormatUint(n.ID, 10) + "&node_type=" + n.UniType
}

// scenarios returns every scenario in report order.
func (e *env) scenarios() ([]*scenario, error) {
	users, nodes := e.ds.Users, e.ds.Nodes
	half := len(users) / 2
	v2Pool, clashPool := users[:half], users[half:]

	subscribe := func(pool []benchUser, query, ua string) func(*rand.Rand, int64) (*http.Request, error) {
		return func(_ *rand.Rand, i int64) (*http.Request, error) {
			u := pool[i%int64(len(pool))]
			return e.get("/api/v1/client/subscribe?token="+u.Token+query, map[string]string{"User-Agent": ua, "Accept-Encoding": "gzip"})
		}
	}
	const v2Query, clashQuery = "&flag=v2ray", "&flag=clash-meta&profile=" + ruleTemplate
	const v2UA, clashUA = "v2rayN/7.10", "mihomo/1.19.27"

	// UniProxy user list ETags, learned by the probe of the 304 scenario.
	etags := make([]string, len(nodes))
	nodeUA := map[string]string{"User-Agent": "V2bX/bench", "Accept-Encoding": e.nodeEncoding}

	// Push bodies: batch consecutive users per report, starting at different offsets.
	bodies := make([][]byte, 16)
	for k := range bodies {
		m := make(map[string][2]int64, e.pushBatch)
		for j := 0; j < e.pushBatch; j++ {
			u := users[(k*e.pushBatch+j)%len(users)]
			m[strconv.FormatUint(u.ID, 10)] = [2]int64{1024, 4096}
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		bodies[k] = b
	}
	push := func(n benchNode, body []byte) (*http.Request, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.base+"/api/v1/server/UniProxy/push"+e.nodeQuery(n), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "V2bX/bench")
		req.Header.Set("Accept-Encoding", "gzip")
		return req, nil
	}

	login := func(i int64) (*http.Request, error) {
		u := users[i%int64(len(users))]
		body := `{"email":"` + u.Email + `","password":"` + benchUserPassword + `"}`
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.base+"/api/v1/passport/login", strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Encoding", "gzip")
		// One client address per request from 198.18.0.0/15 (RFC 2544 benchmarking range): the panel runs behind
		// a reverse proxy it trusts, and the 30/min per-IP passport limit stays in force for each address.
		ip := uint32(i) & 0x1ffff
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.%d.%d.%d", 18+(ip>>16), (ip>>8)&0xff, ip&0xff))
		return req, nil
	}

	browser := map[string]string{"User-Agent": "Mozilla/5.0 (Macintosh; bench)", "Accept-Encoding": "br, gzip"}
	var page []byte // index.html, read by the static_index probe
	asset := ""     // the largest JS chunk the page references, picked by the static_asset probe
	assetRe := regexp.MustCompile(`/?assets/[A-Za-z0-9_.-]+\.js`)
	assetScn := &scenario{
		ID: "static_asset", Group: groupStatic, Title: "largest JS chunk (brotli, immutable)", Request: "GET /assets/<name>-<hash>.js",
		Conc: e.conc, Expect: []int{200},
		Make: func(*rand.Rand, int64) (*http.Request, error) { return e.get(asset, browser) },
	}
	assetScn.Probe = func(_ context.Context, s *scenario) error {
		if page == nil {
			return errors.New("static_asset needs static_index to run first (it reads the page)")
		}
		var best int64 = -1
		seen := map[string]bool{}
		for _, m := range assetRe.FindAll(page, -1) {
			p := "/" + strings.TrimPrefix(string(m), "/")
			if seen[p] {
				continue
			}
			seen[p] = true
			req, err := e.get(p, browser)
			if err != nil {
				return err
			}
			code, h, n, err := e.rawProbe(req)
			if err != nil {
				return fmt.Errorf("GET %s: %w", p, err)
			}
			if code != 200 || h.Get("Content-Encoding") != "br" || !strings.Contains(h.Get("Cache-Control"), "immutable") {
				return fmt.Errorf("GET %s: %d Content-Encoding=%q Cache-Control=%q", p, code,
					h.Get("Content-Encoding"), h.Get("Cache-Control"))
			}
			if n > best {
				best, asset = n, p
			}
		}
		if asset == "" {
			return errors.New("GET /: the page references no /assets/*.js")
		}
		s.Request = "GET " + asset
		name := assetName(asset)
		s.Detail = fmt.Sprintf("GET /assets/%s-*.js · %s on the wire", name, fmtBytes(float64(best)))
		return nil
	}

	return []*scenario{
		{
			ID: "sub_v2ray", Group: groupSub, Title: "v2ray (base64)", Request: "GET /api/v1/client/subscribe?flag=v2ray",
			Detail: "GET /client/subscribe · flag=v2ray", Conc: e.conc, Expect: []int{200}, Budget: int64(len(v2Pool) * e.perToken),
			Make: subscribe(v2Pool, v2Query, v2UA),
			Probe: func(_ context.Context, scn *scenario) error {
				req, err := subscribe(v2Pool, v2Query, v2UA)(nil, 0)
				if err != nil {
					return err
				}
				code, _, body, err := e.probe(req)
				if err != nil {
					return fmt.Errorf("v2ray subscription: %w", err)
				}
				if code != 200 {
					return fmt.Errorf("v2ray subscription: %d %s", code, snippet(body, 200))
				}
				raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
				if err != nil {
					return fmt.Errorf("v2ray subscription is not base64: %w", err)
				}
				links := strings.Count(string(raw), "://")
				if links < len(nodes) {
					return fmt.Errorf("v2ray subscription has %d links, want >= %d", links, len(nodes))
				}
				scn.Note = fmt.Sprintf("plaintext body %s: base64 of %d share links (%d nodes + %d traffic / expiry info entries)",
					fmtBytes(float64(len(body))), links, len(nodes), links-len(nodes))
				return nil
			},
		},
		{
			ID: "sub_clash_rules", Group: groupSub, Title: "mihomo + standard rules", Request: "GET /api/v1/client/subscribe?flag=clash-meta&profile=standard",
			Conc: e.conc, Expect: []int{200}, Budget: int64(len(clashPool) * e.perToken),
			Make: subscribe(clashPool, clashQuery, clashUA),
			Probe: func(_ context.Context, scn *scenario) error {
				req, err := subscribe(clashPool, clashQuery, clashUA)(nil, 0)
				if err != nil {
					return err
				}
				code, h, body, err := e.probe(req)
				if err != nil {
					return fmt.Errorf("mihomo subscription: %w", err)
				}
				if code != 200 {
					return fmt.Errorf("mihomo subscription: %d %s", code, snippet(body, 200))
				}
				s := string(body)
				for _, want := range []string{"proxies:", "proxy-groups:", "rule-providers:", "rules:"} {
					if !strings.Contains(s, want) {
						return fmt.Errorf("mihomo subscription misses %q", want)
					}
				}
				if h.Get("ETag") == "" || h.Get("Subscription-Userinfo") == "" {
					return errors.New("mihomo subscription misses ETag or Subscription-Userinfo")
				}
				for _, n := range nodes {
					if !strings.Contains(s, n.Name) {
						return fmt.Errorf("mihomo subscription misses node %q", n.Name)
					}
				}
				providers := strings.Count(s, "behavior:")
				scn.Detail = fmt.Sprintf("GET /client/subscribe · %d rule providers", providers)
				scn.Note = fmt.Sprintf("plaintext body %s: all %d nodes as proxies (plus the info entries), %d rule providers "+
					"(catalog template %q)", fmtBytes(float64(len(body))), len(nodes), providers, ruleTemplate)
				return nil
			},
		},
		{
			ID: "node_users_304", Group: groupNode, Title: "user list, If-None-Match → 304", Request: "GET /api/v1/server/UniProxy/user + If-None-Match",
			Detail: "GET /UniProxy/user · If-None-Match", Conc: e.conc, Expect: []int{200, 304}, Hit: http.StatusNotModified,
			Make: func(rng *rand.Rand, _ int64) (*http.Request, error) {
				k := rng.IntN(len(nodes))
				req, err := e.get("/api/v1/server/UniProxy/user"+e.nodeQuery(nodes[k]), nodeUA)
				if err == nil {
					req.Header.Set("If-None-Match", etags[k])
				}
				return req, err
			},
			Probe: func(context.Context, *scenario) error {
				for k, n := range nodes {
					req, err := e.get("/api/v1/server/UniProxy/user"+e.nodeQuery(n), nodeUA)
					if err != nil {
						return err
					}
					code, h, body, err := e.probe(req)
					if err != nil {
						return fmt.Errorf("user list of node %d: %w", n.ID, err)
					}
					if code != 200 {
						return fmt.Errorf("user list of node %d: %d %s", n.ID, code, snippet(body, 200))
					}
					var out struct {
						Users []struct {
							ID uint64 `json:"id"`
						} `json:"users"`
					}
					if err := json.Unmarshal(body, &out); err != nil {
						return fmt.Errorf("user list of node %d: %w", n.ID, err)
					}
					if len(out.Users) < len(users) {
						return fmt.Errorf("user list of node %d has %d users, want >= %d", n.ID, len(out.Users), len(users))
					}
					if etags[k] = h.Get("ETag"); etags[k] == "" {
						return fmt.Errorf("user list of node %d has no ETag", n.ID)
					}
				}
				return nil
			},
		},
		{
			ID: "node_users_full", Group: groupNode, Title: fmt.Sprintf("user list, full body (%s users)", groupThousands(int64(len(users)))),
			Request: "GET /api/v1/server/UniProxy/user",
			Detail:  "GET /UniProxy/user · " + e.nodeEncoding,
			Conc:    e.conc, Expect: []int{200},
			Make: func(rng *rand.Rand, _ int64) (*http.Request, error) {
				return e.get("/api/v1/server/UniProxy/user"+e.nodeQuery(nodes[rng.IntN(len(nodes))]), nodeUA)
			},
			Probe: func(_ context.Context, scn *scenario) error {
				req, err := e.get("/api/v1/server/UniProxy/user"+e.nodeQuery(nodes[0]), nodeUA)
				if err != nil {
					return err
				}
				code, _, body, err := e.probe(req)
				if err != nil {
					return fmt.Errorf("user list: %w", err)
				}
				if code != 200 {
					return fmt.Errorf("user list: %d", code)
				}
				how := "cached body sent as is"
				if e.nodeEncoding == "gzip" {
					how = "cached body sent with its cached gzip encoding, compressed once per body version"
				}
				scn.Note = fmt.Sprintf("plaintext JSON %s per response (%s)", fmtBytes(float64(len(body))), how)
				return nil
			},
		},
		{
			ID: "node_push", Group: groupNode, Title: fmt.Sprintf("traffic push (%s users per report)", groupThousands(int64(e.pushBatch))),
			Request: "POST /api/v1/server/UniProxy/push",
			Detail:  "POST /UniProxy/push · flushed every 10 s",
			Conc:    e.conc, Expect: []int{200},
			Make: func(rng *rand.Rand, _ int64) (*http.Request, error) {
				return push(nodes[rng.IntN(len(nodes))], bodies[rng.IntN(len(bodies))])
			},
			Probe: func(context.Context, *scenario) error {
				req, err := push(nodes[0], bodies[0])
				if err != nil {
					return err
				}
				code, _, body, err := e.probe(req)
				if err != nil {
					return fmt.Errorf("push: %w", err)
				}
				if code != 200 || !strings.Contains(string(body), `"data":true`) {
					return fmt.Errorf("push: %d %s", code, snippet(body, 200))
				}
				return nil
			},
			After: e.measureFlush(push),
		},
		{
			ID: "login", Group: groupLogin, Title: "password login (argon2id)", Request: "POST /api/v1/passport/login",
			Detail: "POST /passport/login · 64 MiB, t=3", Conc: e.loginConc, Expect: []int{200},
			Note: "argon2id with 64 MiB, 3 passes, 4 lanes per verification by design; each request comes from its own " +
				"X-Forwarded-For address (server.trusted_proxies = 127.0.0.1), so the 30/min per-IP limit stays on",
			Make: func(_ *rand.Rand, i int64) (*http.Request, error) { return login(i) },
			Probe: func(context.Context, *scenario) error {
				req, err := login(1 << 17) // an address the measurement never uses
				if err != nil {
					return err
				}
				code, _, body, err := e.probe(req)
				if err != nil {
					return fmt.Errorf("login: %w", err)
				}
				if code != 200 || !strings.Contains(string(body), `"token"`) {
					return fmt.Errorf("login: %d %s", code, snippet(body, 200))
				}
				return nil
			},
		},
		{
			ID: "static_index", Group: groupStatic, Title: "index.html (prerendered, brotli)", Request: "GET /",
			Detail: "GET / · no-cache HTML", Conc: e.conc, Expect: []int{200},
			Make: func(*rand.Rand, int64) (*http.Request, error) { return e.get("/", browser) },
			Probe: func(context.Context, *scenario) error {
				req, err := e.get("/", map[string]string{"User-Agent": browser["User-Agent"]})
				if err != nil {
					return err
				}
				code, _, body, err := e.probe(req)
				if err != nil {
					return fmt.Errorf("GET /: %w", err)
				}
				if code != 200 || !strings.Contains(string(body), "/assets/") {
					return fmt.Errorf("GET /: %d %s", code, snippet(body, 200))
				}
				page = body
				req, err = e.get("/", browser)
				if err != nil {
					return err
				}
				code, h, _, err := e.rawProbe(req)
				if err != nil {
					return fmt.Errorf("GET / with br: %w", err)
				}
				if code != 200 || h.Get("Content-Encoding") != "br" {
					return fmt.Errorf("GET / with br: %d Content-Encoding=%q", code, h.Get("Content-Encoding"))
				}
				return nil
			},
		},
		assetScn,
	}, nil
}

// assetName strips the directory, the extension and Vite's "-<8 character hash>" from an asset path; the hash
// alphabet includes '-' and '_' ("/assets/react-B--GpZa_.js" → "react").
func assetName(p string) string {
	name := strings.TrimPrefix(p, "/assets/")
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	if n := len(name); n > 9 && name[n-9] == '-' {
		name = name[:n-9]
	}
	return name
}

// flushState is the traffic_flush entry of /admin/system/status.
type flushState struct {
	Running      bool       `json:"running"`
	LastRun      *time.Time `json:"last_run"` // start of the last completed run
	LastDuration string     `json:"last_duration"`
	LastError    string     `json:"last_error"`
	NextRun      *time.Time `json:"next_run"`
}

func (e *env) flushJob(ctx context.Context) (*flushState, error) {
	var out struct {
		Data struct {
			Scheduler []struct {
				Name string `json:"name"`
				flushState
			} `json:"scheduler"`
		} `json:"data"`
	}
	if err := e.admin.call(ctx, http.MethodGet, "/api/v1/admin/system/status", nil, &out); err != nil {
		return nil, err
	}
	for _, j := range out.Data.Scheduler {
		if j.Name == "traffic_flush" {
			if j.LastError != "" {
				return nil, fmt.Errorf("traffic_flush failed: %s", j.LastError)
			}
			return &j.flushState, nil
		}
	}
	return nil, errors.New("no traffic_flush job in /admin/system/status")
}

// waitFlush waits for a completed traffic_flush run that started at or after since and returns its state.
func (e *env) waitFlush(ctx context.Context, since time.Time) (*flushState, error) {
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		st, err := e.flushJob(ctx)
		if err != nil {
			return nil, err
		}
		if !st.Running && st.LastRun != nil && !st.LastRun.Before(since) {
			return st, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("traffic_flush did not complete within 120 s")
}

// measureFlush drains the accumulator left by the push scenario, then times one controlled flush window: every
// seeded user on every seeded node, pushed between two scheduler ticks, and the traffic_flush run that writes it.
func (e *env) measureFlush(push func(benchNode, []byte) (*http.Request, error)) func(context.Context, *ScenarioResult) error {
	return func(ctx context.Context, res *ScenarioResult) error {
		e.logf("  draining the traffic accumulator")
		if _, err := e.waitFlush(ctx, time.Now()); err != nil {
			return err
		}
		users, nodes := e.ds.Users, e.ds.Nodes
		var reqs [][]byte
		for lo := 0; lo < len(users); lo += e.pushBatch {
			m := map[string][2]int64{}
			for _, u := range users[lo:min(lo+e.pushBatch, len(users))] {
				m[strconv.FormatUint(u.ID, 10)] = [2]int64{1024, 4096}
			}
			b, err := json.Marshal(m)
			if err != nil {
				return err
			}
			reqs = append(reqs, b)
		}
		pairs := int64(len(users) * len(nodes))
		for attempt := 1; attempt <= 3; attempt++ {
			// Start right after a tick so the whole window lands in one run.
			if _, err := e.waitFlush(ctx, time.Now()); err != nil {
				return err
			}
			start := time.Now()
			errc := make(chan error, len(nodes))
			sem := make(chan struct{}, 16)
			for _, n := range nodes {
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()
					for _, b := range reqs {
						req, err := push(n, b)
						if err != nil {
							errc <- err
							return
						}
						code, _, _, err := e.rawProbe(req)
						if err != nil {
							errc <- fmt.Errorf("flush window push: %w", err)
							return
						}
						if code != 200 {
							errc <- fmt.Errorf("flush window push: %d", code)
							return
						}
					}
					errc <- nil
				}()
			}
			for range nodes {
				if err := <-errc; err != nil {
					return err
				}
			}
			pushed := time.Now()
			st, err := e.flushJob(ctx)
			if err != nil {
				return err
			}
			if st.Running || (st.LastRun != nil && !st.LastRun.Before(start)) {
				e.logf("  a flush started while pushing (attempt %d); retrying", attempt)
				continue
			}
			e.logf("  pushed %s (node, user) pairs in %s; waiting for traffic_flush", groupThousands(pairs),
				pushed.Sub(start).Round(time.Millisecond))
			st, err = e.waitFlush(ctx, pushed)
			if err != nil {
				return err
			}
			if d, err := time.ParseDuration(st.LastDuration); err == nil {
				st.LastDuration = d.Round(time.Millisecond).String()
			}
			e.flush, e.flushPairs = st.LastDuration, pairs
			res.Note = fmt.Sprintf("one flush window of %s (node, user) pairs (%d nodes × %s users) was written by the "+
				"next traffic_flush (users.u/d updates plus stat_users and stat_nodes upserts in one transaction) in %s",
				groupThousands(pairs), len(nodes), groupThousands(int64(len(users))), st.LastDuration)
			return nil
		}
		return errors.New("could not push a whole flush window between two traffic_flush ticks")
	}
}
