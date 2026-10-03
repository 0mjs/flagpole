// Package e2e runs Flagpole end to end: the HTTP app and the worker,
// in-process, against the Postgres, Redis and Redpanda from
// docker-compose.yml, in a database and Redis DB of their own. It skips
// when they aren't running.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/0mjs/flagpole/api/internal/platform/config"
	"github.com/0mjs/flagpole/api/internal/platform/kafka"
	"github.com/0mjs/flagpole/api/internal/server"
	"github.com/0mjs/flagpole/api/internal/worker"
)

const (
	adminURL = "postgres://flagpole:flagpole@localhost:5432/flagpole?sslmode=disable"
	testURL  = "postgres://flagpole:flagpole@localhost:5432/flagpole_test?sslmode=disable"
)

// mailbox keeps the emails the app sends, so tests can follow the links.
type mailbox struct {
	mu   sync.Mutex
	sent map[string]string // to -> last body
}

func (m *mailbox) Send(to, _, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent[to] = body
	return nil
}

var tokenPattern = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// token waits for an email to arrive and returns the token in its link.
func (m *mailbox) token(t *testing.T, to string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		body := m.sent[to]
		m.mu.Unlock()
		if match := tokenPattern.FindStringSubmatch(body); match != nil {
			return match[1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no email with a token reached %s", to)
	return ""
}

// stack is a running Flagpole: the server and the worker.
type stack struct {
	srv  *httptest.Server
	mail *mailbox
}

func start(t *testing.T) *stack {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := resetDatabase(ctx); err != nil {
		t.Skipf("Postgres isn't running (docker compose up -d): %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DatabaseURL = testURL
	cfg.RedisURL = "redis://localhost:6379/15"
	cfg.AutoMigrate = true
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mail := &mailbox{sent: map[string]string{}}

	sys, err := server.Build(ctx, cfg, log, mail)
	if err != nil {
		t.Skipf("the stack isn't running (docker compose up -d): %v", err)
	}
	t.Cleanup(sys.Close)
	if err := sys.Redis.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if err := sys.App.Validate(); err != nil {
		t.Fatalf("app.Validate: %v", err)
	}
	if _, err := sys.Identity.CreateAdmin(ctx, "ada@example.com", "Ada", "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}

	// A consumer group of its own, starting at the end, sees only this
	// run's messages.
	consumer, err := kafka.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup(fmt.Sprintf("flagpole-e2e-%d", time.Now().UnixNano())),
		kgo.ConsumeTopics(kafka.TopicFlagChanges, kafka.TopicExposures),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(consumer.Close)
	w := &worker.Worker{DB: sys.DB, Snapshots: sys.Snapshots, Producer: sys.Producer, Consumer: consumer, Log: log}
	go w.RelayOutbox(ctx, 50*time.Millisecond)
	go w.Consume(ctx)

	srv := httptest.NewServer(sys.App)
	t.Cleanup(srv.Close)
	return &stack{srv: srv, mail: mail}
}

func resetDatabase(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS flagpole_test WITH (FORCE)"); err != nil {
		return err
	}
	_, err = conn.Exec(ctx, "CREATE DATABASE flagpole_test")
	return err
}

// client is a browser for one user: cookies, CSRF header and all.
type client struct {
	t    *testing.T
	base string
	http *http.Client
	key  string // an SDK key, for the SDK API
}

func (s *stack) client(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: s.srv.URL, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

func (s *stack) sdk(t *testing.T, key string) *client {
	return &client{t: t, base: s.srv.URL, http: &http.Client{Timeout: 10 * time.Second}, key: key}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %s: %v", r.body, err)
	}
}

// in returns the client reporting to t, for use inside a subtest.
func (c *client) in(t *testing.T) *client {
	cp := *c
	cp.t = t
	return &cp
}

func (c *client) do(method, path string, body any, headers ...string) response {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.base+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	if c.http.Jar != nil {
		u, _ := url.Parse(c.base)
		for _, ck := range c.http.Jar.Cookies(u) {
			if ck.Name == "flagpole_csrf" {
				req.Header.Set("X-CSRF-Token", ck.Value)
			}
		}
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return response{status: resp.StatusCode, header: resp.Header, body: raw}
}

// expect does a request and fails unless it answers status.
func (c *client) expect(status int, method, path string, body any, out ...any) response {
	c.t.Helper()
	r := c.do(method, path, body)
	if r.status != status {
		c.t.Fatalf("%s %s: %d, want %d: %s", method, path, r.status, status, r.body)
	}
	if len(out) > 0 {
		r.decode(c.t, out[0])
	}
	return r
}

func (c *client) login(email, password string) {
	c.t.Helper()
	c.do("GET", "/api/v1/auth/me", nil) // picks up the CSRF cookie
	c.expect(200, "POST", "/api/v1/auth/login", map[string]string{"email": email, "password": password})
}

// invite has admin invite someone and returns them signed in.
func (s *stack) invite(t *testing.T, admin *client, email, role string) *client {
	t.Helper()
	admin.expect(201, "POST", "/api/v1/users", map[string]string{"email": email, "name": strings.Split(email, "@")[0], "role": role})
	c := s.client(t)
	c.do("GET", "/api/v1/auth/me", nil)
	c.expect(200, "POST", "/api/v1/auth/invites/accept", map[string]string{"token": s.mail.token(t, email), "password": "a-long-enough-password"})
	return c
}

type flagConfig struct {
	Environment    string `json:"environment"`
	Enabled        bool   `json:"enabled"`
	DefaultVariant string `json:"default_variant"`
	OffVariant     string `json:"off_variant"`
	Rules          []any  `json:"rules"`
	Version        int    `json:"version"`
}

type changeRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func TestFlagpole(t *testing.T) {
	s := start(t)
	admin := s.client(t)
	admin.login("ada@example.com", "correct-horse-battery")

	editor := s.invite(t, admin, "grace@example.com", "editor")
	approver := s.invite(t, admin, "linus@example.com", "approver")
	viewer := s.invite(t, admin, "vera@example.com", "viewer")

	t.Run("sign-in", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		anon := s.client(t)
		anon.expect(401, "GET", "/api/v1/auth/me", nil)
		anon.do("GET", "/api/v1/auth/me", nil)
		r := anon.do("POST", "/api/v1/auth/login", map[string]string{"email": "grace@example.com", "password": "wrong"})
		if r.status != 401 || !strings.Contains(string(r.body), "invalid email or password") {
			t.Fatalf("wrong password: %d %s", r.status, r.body)
		}
		// Without the CSRF header, a change is refused even with a session.
		raw := &client{t: t, base: s.srv.URL, http: editor.http}
		req, _ := http.NewRequest("POST", raw.base+"/api/v1/auth/logout", nil)
		resp, err := raw.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("logout without CSRF token: %d", resp.StatusCode)
		}
	})

	var project struct {
		Key          string `json:"key"`
		Environments []struct {
			Key              string `json:"key"`
			RequiresApproval bool   `json:"requires_approval"`
		} `json:"environments"`
	}
	t.Run("projects", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		editor.expect(403, "POST", "/api/v1/projects", map[string]string{"key": "web", "name": "Web"})
		r := admin.expect(201, "POST", "/api/v1/projects", map[string]string{"key": "web", "name": "Web app"}, &project)
		if r.header.Get("Location") != "/api/v1/projects/web" || len(project.Environments) != 3 || !project.Environments[2].RequiresApproval {
			t.Fatalf("project: %s %s", r.header.Get("Location"), r.body)
		}
		admin.expect(409, "POST", "/api/v1/projects", map[string]string{"key": "web", "name": "Again"})
		admin.expect(422, "POST", "/api/v1/projects", map[string]string{"key": "Not A Slug", "name": "Bad"})
		viewer.expect(404, "GET", "/api/v1/projects/nope", nil)
	})

	const flags = "/api/v1/projects/web/flags"
	t.Run("flags", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		viewer.expect(403, "POST", flags, map[string]any{"key": "new-checkout", "name": "New checkout", "kind": "boolean"})
		var flag struct {
			Variants     []map[string]any `json:"variants"`
			Environments []flagConfig     `json:"environments"`
		}
		r := editor.expect(201, "POST", flags, map[string]any{"key": "new-checkout", "name": "New checkout", "kind": "boolean", "tags": []string{"checkout"}}, &flag)
		if len(flag.Variants) != 2 || len(flag.Environments) != 3 || flag.Environments[0].Enabled || r.header.Get("Location") == "" {
			t.Fatalf("flag: %s", r.body)
		}
		editor.expect(409, "POST", flags, map[string]any{"key": "new-checkout", "name": "Again", "kind": "boolean"})
		// The key's pattern is checked as the request binds; the variants,
		// which no tag can describe, by the service.
		r = editor.do("POST", flags, map[string]any{"key": "Bad Key", "name": "x", "kind": "boolean"})
		if r.status != 422 || !strings.Contains(string(r.body), `"key":"must match the pattern`) {
			t.Fatalf("bad key: %d %s", r.status, r.body)
		}
		r = editor.do("POST", flags, map[string]any{"key": "odd-variants", "name": "x", "kind": "string", "variants": []map[string]any{{"key": "a", "value": 1}}})
		if r.status != 422 || !strings.Contains(string(r.body), `"variants"`) {
			t.Fatalf("bad variants: %d %s", r.status, r.body)
		}
		editor.expect(201, "POST", flags, map[string]any{"key": "button-color", "name": "Button color", "kind": "string",
			"variants": []map[string]any{{"key": "blue", "value": "blue"}, {"key": "green", "value": "green"}}})
	})

	staging := flags + "/new-checkout/environments/staging"
	production := flags + "/new-checkout/environments/production"
	on := func(version int) map[string]any {
		return map[string]any{"enabled": true, "default_variant": "off", "off_variant": "off", "base_version": version,
			"rules": []map[string]any{{"conditions": []map[string]any{{"attribute": "plan", "operator": "in", "values": []string{"pro"}}}, "variant": "on"}}}
	}

	t.Run("configs", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		var cfg flagConfig
		editor.expect(200, "PUT", staging, on(1), &cfg)
		if !cfg.Enabled || cfg.Version != 2 || len(cfg.Rules) != 1 {
			t.Fatalf("staging: %+v", cfg)
		}
		r := editor.do("PUT", staging, on(1))
		if r.status != 409 || !strings.Contains(string(r.body), "version 2, not 1") {
			t.Fatalf("stale version: %d %s", r.status, r.body)
		}
		bad := on(2)
		bad["rules"] = []map[string]any{{"rollout": []map[string]any{{"variant": "on", "weight": 60}}}}
		r = editor.do("PUT", staging, bad)
		if r.status != 422 || !strings.Contains(string(r.body), "add up to 60") {
			t.Fatalf("bad rollout: %d %s", r.status, r.body)
		}
		r = editor.do("PUT", production, on(1))
		if r.status != 409 || !strings.Contains(string(r.body), "requires approval") {
			t.Fatalf("production without review: %d %s", r.status, r.body)
		}
	})

	t.Run("change requests", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		var cr changeRequest
		editor.expect(201, "POST", production+"/change-requests", on(1), &cr)
		editor.expect(409, "POST", production+"/change-requests", on(1))
		changes := "/api/v1/projects/web/change-requests/"
		editor.expect(403, "POST", changes+cr.ID+"/approve", map[string]string{})
		approver.expect(200, "POST", changes+cr.ID+"/approve", map[string]string{"comment": "ship it"}, &cr)
		if cr.Status != "applied" {
			t.Fatalf("approved: %+v", cr)
		}
		var cfg flagConfig
		viewer.expect(200, "GET", production, nil, &cfg)
		if !cfg.Enabled || cfg.Version != 2 {
			t.Fatalf("production after approval: %+v", cfg)
		}

		// An approver can't approve their own change.
		off := map[string]any{"enabled": false, "default_variant": "off", "off_variant": "off", "rules": []any{}, "base_version": 2}
		approver.expect(201, "POST", production+"/change-requests", off, &cr)
		r := approver.do("POST", changes+cr.ID+"/approve", map[string]string{})
		if r.status != 403 || !strings.Contains(string(r.body), "your own change") {
			t.Fatalf("self-approval: %d %s", r.status, r.body)
		}
		approver.expect(200, "POST", changes+cr.ID+"/cancel", nil)

		// A change that's overtaken is marked conflicted, not applied.
		editor.expect(201, "POST", production+"/change-requests", off, &cr)
		admin.expect(200, "PATCH", "/api/v1/projects/web/environments/production", map[string]any{"name": "Production", "requires_approval": false})
		editor.expect(200, "PUT", production, on(2))
		admin.expect(200, "PATCH", "/api/v1/projects/web/environments/production", map[string]any{"name": "Production", "requires_approval": true})
		approver.expect(409, "POST", changes+cr.ID+"/approve", map[string]string{})
		approver.expect(200, "GET", changes+cr.ID, nil, &cr)
		if cr.Status != "conflicted" {
			t.Fatalf("overtaken change: %+v", cr)
		}
	})

	var key struct {
		Key string `json:"key"`
		ID  string `json:"id"`
	}
	t.Run("sdk", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		admin.expect(201, "POST", "/api/v1/projects/web/environments/production/sdk-keys", map[string]string{"name": "checkout"}, &key)
		sdk := s.sdk(t, key.Key)
		s.sdk(t, "").expect(401, "GET", "/sdk/v1/flags", nil)
		s.sdk(t, "fp_pro_nope").expect(401, "GET", "/sdk/v1/flags", nil)

		var snap struct {
			Version string                    `json:"version"`
			Flags   map[string]map[string]any `json:"flags"`
		}
		r := sdk.expect(200, "GET", "/sdk/v1/flags", nil, &snap)
		if r.header.Get("ETag") != `"`+snap.Version+`"` || len(snap.Flags) != 2 {
			t.Fatalf("snapshot: %v %s", r.header, r.body)
		}
		if r := sdk.do("GET", "/sdk/v1/flags", nil, "If-None-Match", r.header.Get("ETag")); r.status != 304 {
			t.Fatalf("If-None-Match: %d", r.status)
		}

		var eval struct {
			Flags map[string]struct {
				Variant string `json:"variant"`
				Reason  string `json:"reason"`
			} `json:"flags"`
		}
		sdk.expect(200, "POST", "/sdk/v1/evaluate", map[string]any{"context": map[string]any{"key": "u1", "attributes": map[string]any{"plan": "pro"}}}, &eval)
		if got := eval.Flags["new-checkout"]; got.Variant != "on" || got.Reason != "rule" {
			t.Fatalf("pro user: %+v", eval.Flags)
		}
		sdk.expect(200, "POST", "/sdk/v1/evaluate", map[string]any{"context": map[string]any{"key": "u2"}, "flags": []string{"new-checkout", "missing"}}, &eval)
		if eval.Flags["new-checkout"].Variant != "off" || eval.Flags["missing"].Reason != "not_found" {
			t.Fatalf("free user: %+v", eval.Flags)
		}
	})

	t.Run("stream", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		req, _ := http.NewRequest("GET", s.srv.URL+"/sdk/v1/stream", nil)
		req.Header.Set("Authorization", "Bearer "+key.Key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		events := make(chan string, 10)
		go func() {
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				if name, ok := strings.CutPrefix(scanner.Text(), "event: "); ok {
					events <- name
				}
			}
		}()
		if got := <-events; got != "ready" {
			t.Fatalf("first event: %s", got)
		}
		// A change goes outbox -> Kafka -> worker -> Redis -> stream.
		var cr changeRequest
		editor.expect(201, "POST", flags+"/button-color/environments/production/change-requests", map[string]any{
			"enabled": true, "default_variant": "green", "off_variant": "blue", "rules": []any{}, "base_version": 1,
		}, &cr)
		approver.expect(200, "POST", "/api/v1/projects/web/change-requests/"+cr.ID+"/approve", map[string]string{})
		select {
		case got := <-events:
			if got != "changed" {
				t.Fatalf("event: %s", got)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("no changed event within 10s")
		}
	})

	t.Run("exposures", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		sdk := s.sdk(t, key.Key)
		sdk.expect(202, "POST", "/sdk/v1/exposures", map[string]any{"exposures": []map[string]string{
			{"flag": "new-checkout", "variant": "on"}, {"flag": "new-checkout", "variant": "on"},
			{"flag": "new-checkout", "variant": "off"}, {"flag": "not-a-flag", "variant": "on"},
		}})
		deadline := time.Now().Add(10 * time.Second)
		for {
			var stats struct {
				Totals map[string]int `json:"totals"`
			}
			viewer.expect(200, "GET", production+"/exposures", nil, &stats)
			if stats.Totals["on"] == 2 && stats.Totals["off"] == 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("exposures not counted within 10s: %+v", stats.Totals)
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("revoke and archive", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		sdk := s.sdk(t, key.Key)
		admin.expect(204, "DELETE", "/api/v1/projects/web/environments/production/sdk-keys/"+key.ID, nil)
		sdk.expect(401, "GET", "/sdk/v1/flags", nil)

		admin.expect(201, "POST", "/api/v1/projects/web/environments/production/sdk-keys", map[string]string{"name": "again"}, &key)
		editor.expect(200, "POST", flags+"/button-color/archive", nil)
		var snap struct {
			Flags map[string]any `json:"flags"`
		}
		s.sdk(t, key.Key).expect(200, "GET", "/sdk/v1/flags", nil, &snap)
		if _, ok := snap.Flags["button-color"]; ok || len(snap.Flags) != 1 {
			t.Fatalf("archived flag still served: %v", snap.Flags)
		}
	})

	t.Run("users and audit", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		var me struct{ ID string }
		admin.expect(200, "GET", "/api/v1/auth/me", nil, &me)
		admin.expect(403, "PATCH", "/api/v1/users/"+me.ID, map[string]string{"role": "viewer"})
		var users []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		admin.expect(200, "GET", "/api/v1/users", nil, &users)
		var grace string
		for _, u := range users {
			if u.Email == "grace@example.com" {
				grace = u.ID
			}
		}
		admin.expect(200, "POST", "/api/v1/users/"+grace+"/disable", nil)
		editor.expect(401, "GET", "/api/v1/auth/me", nil)

		var page struct {
			Events []struct {
				Action  string `json:"action"`
				Summary string `json:"summary"`
			} `json:"events"`
		}
		viewer.expect(200, "GET", "/api/v1/projects/web/audit?limit=200", nil, &page)
		actions := map[string]bool{}
		for _, e := range page.Events {
			actions[e.Action] = true
		}
		for _, want := range []string{"project.created", "flag.created", "flag.config_changed", "change.requested", "change.conflicted", "change.cancelled", "sdk_key.revoked", "flag.archived"} {
			if !actions[want] {
				t.Errorf("audit lacks %s: %v", want, actions)
			}
		}
		viewer.expect(403, "GET", "/api/v1/audit", nil)
	})

	// The spec says which role each route needs, as a scope on the session
	// scheme. Check the guards agree: the role below is refused, and a viewer
	// can read every route that names no role.
	t.Run("spec matches guards", func(t *testing.T) {
		admin, approver, viewer := admin.in(t), approver.in(t), viewer.in(t)
		// The editor was disabled above, which ended their session.
		var users []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		admin.expect(200, "GET", "/api/v1/users", nil, &users)
		for _, u := range users {
			if u.Email == "grace@example.com" {
				admin.expect(200, "POST", "/api/v1/users/"+u.ID+"/enable", nil)
			}
		}
		editor := s.client(t)
		editor.login("grace@example.com", "a-long-enough-password")
		below := map[string]*client{"editor": viewer, "approver": editor, "admin": approver}
		var spec struct {
			Paths map[string]map[string]struct {
				Security []map[string][]string `json:"security"`
			} `json:"paths"`
		}
		admin.expect(200, "GET", "/openapi.json", nil, &spec)
		params := strings.NewReplacer("{project}", project.Key, "{env}", "development", "{flag}", "no-such-flag", "{id}", "00000000-0000-0000-0000-000000000000")
		scoped, open := 0, 0
		for path, ops := range spec.Paths {
			for method, op := range ops {
				method = strings.ToUpper(method)
				for _, req := range op.Security {
					scopes, ok := req["session"]
					if !ok {
						continue
					}
					if len(scopes) == 0 {
						if method == "GET" {
							open++
							if r := viewer.do(method, params.Replace(path), nil); r.status == 403 {
								t.Errorf("GET %s names no role, but a viewer got 403", path)
							}
						}
						continue
					}
					scoped++
					if r := below[scopes[0]].do(method, params.Replace(path), map[string]any{}); r.status != 403 {
						t.Errorf("%s %s needs %s, but the role below got %d: %s", method, path, scopes[0], r.status, r.body)
					}
				}
			}
		}
		if scoped < 20 || open < 10 {
			t.Fatalf("checked %d scoped and %d open routes; the spec looks wrong", scoped, open)
		}
		t.Logf("%d scoped routes refuse the role below; a viewer reads all %d open GETs", scoped, open)
	})

	t.Run("login rate limit", func(t *testing.T) {
		admin, editor, approver, viewer := admin.in(t), editor.in(t), approver.in(t), viewer.in(t)
		_, _, _, _ = admin, editor, approver, viewer
		c := s.client(t)
		c.do("GET", "/api/v1/auth/me", nil)
		var last response
		for range 11 {
			last = c.do("POST", "/api/v1/auth/login", map[string]string{"email": "vera@example.com", "password": "guess"})
		}
		if last.status != 429 || last.header.Get("Retry-After") == "" {
			t.Fatalf("11th attempt: %d %v %s", last.status, last.header, last.body)
		}
	})
}
