// Package server assembles the HTTP app: middleware, the modules' routes,
// and the OpenAPI description, in one place.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/cors"
	"github.com/0mjs/zinc/middleware/csrf"
	"github.com/0mjs/zinc/middleware/healthcheck"
	"github.com/0mjs/zinc/middleware/logger"
	"github.com/0mjs/zinc/middleware/prometheus"
	"github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/requestid"
	"github.com/0mjs/zinc/middleware/secure"
	"github.com/0mjs/zinc/middleware/session"
	"github.com/0mjs/zinc/middleware/timeout"

	"github.com/0mjs/flagpole/api/internal/evaluation"
	"github.com/0mjs/flagpole/api/internal/flags"
	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/insights"
	"github.com/0mjs/flagpole/api/internal/platform/config"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/projects"
)

// Deps are what the routes need.
type Deps struct {
	Config   config.Config
	DB       *postgres.DB
	Redis    *redis.Client
	Log      *slog.Logger
	Identity *identity.Service
	Projects *projects.Service
	Flags    *flags.Service
	Insights *insights.Service
	SDK      *evaluation.API
}

const sessionCookie = "flagpole_session"

// New builds the app. Call app.Validate before serving it.
func New(d Deps) *zinc.App {
	app := zinc.New(zinc.Config{
		ErrorHandler:      zinc.ProblemErrors,
		BodyLimit:         1 << 20,
		ValidateResponses: !d.Config.Production(),
		TrustedProxies:    []string{"127.0.0.1/32", "::1/128"},
		OpenAPI: zinc.OpenAPIConfig{
			Title:       "Flagpole",
			Version:     "0.1.0",
			License:     &zinc.OpenAPILicense{Name: "MIT", Identifier: "MIT"},
			Servers:     []zinc.OpenAPIServer{{URL: "/", Description: "The server this spec came from"}},
			Description: "Feature flags with roles, reviewed changes for protected environments, and an SDK API.",
			Tags: []zinc.OpenAPITag{
				{Name: "auth", Description: "Signing in and out, invites and password resets."},
				{Name: "users", Description: "Managing users and their roles. Admins only."},
				{Name: "projects", Description: "Projects group flags. Every project has environments."},
				{Name: "environments", Description: "Development, staging, production... Production requires approval by default."},
				{Name: "sdk-keys", Description: "Keys services use to read an environment's flags. Admins only."},
				{Name: "flags", Description: "Flags, their variants, and how each behaves per environment."},
				{Name: "change-requests", Description: "Reviewed changes for environments that require approval."},
				{Name: "audit", Description: "Who did what."},
				{Name: "sdk", Description: "The API services call with an SDK key."},
			},
			SecuritySchemes: map[string]zinc.OpenAPISecurityScheme{
				"session": {Type: "apiKey", In: "cookie", Name: sessionCookie, Description: "Set by POST /api/v1/auth/login. " +
					"Scopes are roles, lowest first: viewer, editor, approver, admin; a route that needs editor lets approvers and admins in too."},
				"sdkKey": {Type: "http", Scheme: "bearer", Description: "An SDK key, from the dashboard: Authorization: Bearer fp_..."},
			},
		},
	})

	metrics := prometheus.NewMetrics(2000)
	app.Use(
		requestid.New(),
		logger.New(logger.Config{Logger: d.Log}),
		recover.New(),
		secure.New(),
		prometheus.New(prometheus.Config{Metrics: metrics}),
		healthcheck.New(healthcheck.Config{Check: func(c *zinc.Context) error {
			if err := d.DB.Pool.Ping(c.Context()); err != nil {
				return err
			}
			return d.Redis.Ping(c.Context()).Err()
		}}),
	)
	app.Get("/metrics", prometheus.Handler(metrics)).Hidden()

	// The dashboard API: cookie sessions, so CSRF protection too. The web
	// app reads the token from the csrf cookie and sends it back as a
	// header on every change.
	api := app.Group("/api/v1",
		timeout.New(timeout.Config{Timeout: 10 * time.Second}),
		session.New(session.Config{
			Name:     sessionCookie,
			Secret:   d.Config.SessionSecret,
			Lifetime: d.Config.SessionLifetime,
			MaxAge:   int(d.Config.SessionLifetime.Seconds()),
			Secure:   d.Config.CookieSecure,
			SameSite: http.SameSiteLaxMode,
			Path:     "/",
		}),
		csrf.New(csrf.Config{
			Cookie:         csrf.Cookie{Name: "flagpole_csrf", Path: "/", Secure: d.Config.CookieSecure, SameSite: http.SameSiteLaxMode},
			TrustedOrigins: []string{d.Config.PublicURL},
			ErrorHandler: func(*zinc.Context, error) error {
				return zinc.Forbidden("missing or invalid CSRF token; send the flagpole_csrf cookie's value in X-CSRF-Token")
			},
		}),
		d.Identity.Authenticate(),
	).Security("session").Document(timeout.Doc(), csrfDoc())

	d.Identity.Routes(api.Group("/auth").Security(), api.Group("/users"))
	project := api.Group("/projects/{project}")
	d.Projects.Routes(api.Group("/projects"), project)
	d.Flags.Routes(project)
	d.Insights.Routes(api.Group("/audit"), project)

	// The SDK API: called by services and browsers with an SDK key, from
	// any origin. No timeout: /stream stays open.
	sdk := app.Group("/sdk/v1",
		cors.New(cors.Config{
			AllowOrigins:  []string{"*"},
			AllowMethods:  []string{http.MethodGet, http.MethodPost},
			AllowHeaders:  []string{"Authorization", "Content-Type", "If-None-Match"},
			ExposeHeaders: []string{"ETag"},
			MaxAge:        3600,
		}),
		d.SDK.RequireSDKKey(),
	).Security("sdkKey")
	d.SDK.Routes(sdk)
	return app
}

// csrfDoc describes the CSRF middleware for the spec. Its error handler
// answers every failure with 403, where the default sends 400 for a missing
// token, so the description says 403 alone.
func csrfDoc() zinc.MiddlewareDoc {
	doc := csrf.Doc()
	doc.Errors = []int{http.StatusForbidden}
	return doc
}
