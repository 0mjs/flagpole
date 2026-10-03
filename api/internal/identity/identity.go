// Package identity is who's who: users and their roles, sign-in with
// sessions, invites, password resets, and the middleware that guards routes
// by role.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/platform/events"
	"github.com/0mjs/flagpole/api/internal/platform/httpx"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/store"
)

// Role is what a user may do. Each role includes the ones before it:
// viewers read, editors change flags, approvers also review changes to
// protected environments, and admins also manage users and projects.
type Role string

const (
	Viewer   Role = "viewer"
	Editor   Role = "editor"
	Approver Role = "approver"
	Admin    Role = "admin"
)

func (Role) Enum() []any { return []any{Viewer, Editor, Approver, Admin} }

func (r Role) rank() int {
	switch r {
	case Editor:
		return 1
	case Approver:
		return 2
	case Admin:
		return 3
	}
	return 0
}

// Allows reports whether r includes min.
func (r Role) Allows(min Role) bool { return r.rank() >= min.rank() }

// UserStatus is where a user is in their lifecycle.
type UserStatus string

func (UserStatus) Enum() []any { return []any{"active", "invited", "disabled"} }

// User is a user as the API shows them. The password hash never leaves
// this package.
type User struct {
	ID        uuid.UUID  `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Role      Role       `json:"role"`
	Status    UserStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}

func toUser(u store.User) User {
	status := UserStatus("active")
	switch {
	case u.DisabledAt != nil:
		status = "disabled"
	case u.PasswordHash == nil:
		status = "invited"
	}
	return User{ID: u.ID, Email: u.Email, Name: u.Name, Role: Role(u.Role), Status: status, CreatedAt: u.CreatedAt}
}

// Mailer sends the invite and reset emails.
type Mailer interface {
	Send(to, subject, body string) error
}

// Service holds the identity rules. Handlers call it; it owns the
// transactions.
type Service struct {
	db        *postgres.DB
	rdb       *redis.Client
	mail      Mailer
	publicURL string
	sessions  sessions
	logins    limiter // per email and IP
	loginsIP  limiter // per IP, across emails
}

func NewService(db *postgres.DB, rdb *redis.Client, sender Mailer, publicURL string, sessionLifetime time.Duration) *Service {
	return &Service{
		db: db, rdb: rdb, mail: sender, publicURL: publicURL,
		sessions: sessions{rdb: rdb, idle: 7 * 24 * time.Hour, lifetime: sessionLifetime},
		logins:   limiter{rdb: rdb, max: 10, window: 15 * time.Minute},
		loginsIP: limiter{rdb: rdb, max: 100, window: 15 * time.Minute},
	}
}

// errBadLogin is the one answer for an unknown email, a wrong password and
// a disabled account, so the response doesn't reveal which accounts exist.
var errBadLogin = zinc.Unauthorized("invalid email or password")

// Login checks the credentials and starts a session, returning its ID.
func (s *Service) Login(ctx context.Context, email, password, ip string) (User, string, error) {
	email = normalizeEmail(email)
	for _, l := range []struct {
		lim *limiter
		key string
	}{{&s.logins, "login:" + email + ":" + ip}, {&s.loginsIP, "login-ip:" + ip}} {
		ok, wait, err := l.lim.allow(ctx, l.key)
		if err != nil {
			return User{}, "", err
		}
		if !ok {
			return User{}, "", zinc.TooManyRequests("too many sign-in attempts; try again later").
				WithHeader(zinc.HeaderRetryAfter, fmt.Sprint(int(wait.Seconds())+1))
		}
	}
	u, err := s.db.GetUserByEmail(ctx, email)
	if postgres.IsNotFound(err) || (err == nil && u.PasswordHash == nil) {
		_, _ = VerifyPassword(password, dummyHash)
		return User{}, "", errBadLogin
	}
	if err != nil {
		return User{}, "", err
	}
	ok, err := VerifyPassword(password, *u.PasswordHash)
	if err != nil {
		return User{}, "", err
	}
	if !ok || u.DisabledAt != nil {
		return User{}, "", errBadLogin
	}
	s.logins.reset(ctx, "login:"+email+":"+ip)
	id, err := s.sessions.create(ctx, u.ID)
	if err != nil {
		return User{}, "", err
	}
	return toUser(u), id, nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionID string) {
	if userID, err := s.sessions.lookup(ctx, sessionID); err == nil {
		s.sessions.delete(ctx, sessionID, userID)
	}
}

// UserForSession returns the signed-in user, or an error when the session
// has ended or the user was disabled since.
func (s *Service) UserForSession(ctx context.Context, sessionID string) (User, error) {
	userID, err := s.sessions.lookup(ctx, sessionID)
	if err != nil {
		return User{}, err
	}
	u, err := s.cachedUser(ctx, userID)
	if err != nil {
		return User{}, err
	}
	if u.Status != "active" {
		s.sessions.delete(ctx, sessionID, userID)
		return User{}, errNoSession
	}
	return u, nil
}

// Users are cached in Redis for a few minutes, and the cache is cleared
// whenever a user changes, so a new role applies on the next request.
func userCacheKey(id uuid.UUID) string { return "user:" + id.String() }

func (s *Service) cachedUser(ctx context.Context, id uuid.UUID) (User, error) {
	if raw, err := s.rdb.Get(ctx, userCacheKey(id)).Bytes(); err == nil {
		var u User
		if json.Unmarshal(raw, &u) == nil {
			return u, nil
		}
	}
	row, err := s.db.GetUser(ctx, id)
	if err != nil {
		return User{}, err
	}
	u := toUser(row)
	if raw, err := json.Marshal(u); err == nil {
		s.rdb.Set(ctx, userCacheKey(id), raw, 5*time.Minute)
	}
	return u, nil
}

func (s *Service) forget(ctx context.Context, id uuid.UUID) {
	s.rdb.Del(ctx, userCacheKey(id))
}

// ListUsers returns every user, oldest first.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]User, len(rows))
	for i, r := range rows {
		users[i] = toUser(r)
	}
	return users, nil
}

// Invite creates a user without a password and emails them a link to set
// one.
func (s *Service) Invite(ctx context.Context, actor User, email, name string, role Role) (User, error) {
	email = normalizeEmail(email)
	var u store.User
	var token string
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		var err error
		u, err = q.CreateUser(ctx, store.CreateUserParams{Email: email, Name: name, Role: store.UserRole(role)})
		if postgres.IsUniqueViolation(err) {
			return httpx.Conflict("a user with email %s already exists", email)
		}
		if err != nil {
			return err
		}
		if token, err = s.issueToken(ctx, q, u.ID, store.TokenPurposeInvite, 7*24*time.Hour, &actor.ID); err != nil {
			return err
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "user.invited", EntityType: "user", EntityID: &u.ID,
			Summary: fmt.Sprintf("%s invited %s as %s", actor.Name, email, role),
		})
	})
	if err != nil {
		return User{}, err
	}
	s.send(email, "You're invited to Flagpole", fmt.Sprintf(
		"%s invited you to Flagpole as %s.\n\nSet your password to sign in:\n%s/accept-invite?token=%s\n\nThe link works for 7 days.\n",
		actor.Name, role, s.publicURL, token))
	return toUser(u), nil
}

// AcceptInvite sets the invited user's password and signs them in.
func (s *Service) AcceptInvite(ctx context.Context, token, password string) (User, string, error) {
	userID, err := s.useToken(ctx, token, store.TokenPurposeInvite, password)
	if err != nil {
		return User{}, "", err
	}
	u, err := s.db.GetUser(ctx, userID)
	if err != nil {
		return User{}, "", err
	}
	id, err := s.sessions.create(ctx, userID)
	return toUser(u), id, err
}

// RequestPasswordReset emails a reset link if the account exists. It
// answers the same either way.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	u, err := s.db.GetUserByEmail(ctx, normalizeEmail(email))
	if postgres.IsNotFound(err) || (err == nil && (u.DisabledAt != nil || u.PasswordHash == nil)) {
		return nil
	}
	if err != nil {
		return err
	}
	var token string
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		if err := q.RevokeUserTokens(ctx, store.RevokeUserTokensParams{UserID: u.ID, Purpose: store.TokenPurposePasswordReset}); err != nil {
			return err
		}
		token, err = s.issueToken(ctx, q, u.ID, store.TokenPurposePasswordReset, time.Hour, nil)
		return err
	})
	if err != nil {
		return err
	}
	s.send(u.Email, "Reset your Flagpole password", fmt.Sprintf(
		"Someone asked to reset the password for %s.\n\nChoose a new one here:\n%s/reset-password?token=%s\n\nThe link works for an hour. If it wasn't you, ignore this email.\n",
		u.Email, s.publicURL, token))
	return nil
}

// ResetPassword sets a new password and signs the user out everywhere.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	userID, err := s.useToken(ctx, token, store.TokenPurposePasswordReset, password)
	if err != nil {
		return err
	}
	return s.sessions.deleteAll(ctx, userID)
}

// ChangeRole sets a user's role. Admins can't change their own, and the last
// admin can't be demoted.
func (s *Service) ChangeRole(ctx context.Context, actor User, id uuid.UUID, role Role) (User, error) {
	if id == actor.ID {
		return User{}, httpx.Forbidden("you can't change your own role")
	}
	var u store.User
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		before, err := q.GetUser(ctx, id)
		if postgres.IsNotFound(err) {
			return httpx.NotFound("user not found")
		}
		if err != nil {
			return err
		}
		if before.Role == store.UserRoleAdmin && role != Admin {
			if err := s.keepAnAdmin(ctx, q); err != nil {
				return err
			}
		}
		if u, err = q.UpdateUserRole(ctx, store.UpdateUserRoleParams{ID: id, Role: store.UserRole(role)}); err != nil {
			return err
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "user.role_changed", EntityType: "user", EntityID: &id,
			Summary: fmt.Sprintf("%s changed %s from %s to %s", actor.Name, u.Email, before.Role, role),
			Data:    map[string]any{"from": before.Role, "to": role},
		})
	})
	s.forget(ctx, id)
	return toUser(u), err
}

// SetDisabled disables or re-enables a user. Disabling ends their sessions.
func (s *Service) SetDisabled(ctx context.Context, actor User, id uuid.UUID, disabled bool) (User, error) {
	if id == actor.ID {
		return User{}, httpx.Forbidden("you can't disable yourself")
	}
	var u store.User
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		before, err := q.GetUser(ctx, id)
		if postgres.IsNotFound(err) {
			return httpx.NotFound("user not found")
		}
		if err != nil {
			return err
		}
		if disabled && before.Role == store.UserRoleAdmin {
			if err := s.keepAnAdmin(ctx, q); err != nil {
				return err
			}
		}
		if u, err = q.SetUserDisabled(ctx, store.SetUserDisabledParams{ID: id, Disabled: disabled}); err != nil {
			return err
		}
		action, verb := "user.enabled", "enabled"
		if disabled {
			action, verb = "user.disabled", "disabled"
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: action, EntityType: "user", EntityID: &id,
			Summary: fmt.Sprintf("%s %s %s", actor.Name, verb, u.Email),
		})
	})
	if err != nil {
		return User{}, err
	}
	s.forget(ctx, id)
	if disabled {
		err = s.sessions.deleteAll(ctx, id)
	}
	return toUser(u), err
}

// SignOutEverywhere ends all of a user's sessions.
func (s *Service) SignOutEverywhere(ctx context.Context, id uuid.UUID) error {
	return s.sessions.deleteAll(ctx, id)
}

// CreateAdmin creates an active admin, for bootstrapping from the CLI.
func (s *Service) CreateAdmin(ctx context.Context, email, name, password string) (User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	u, err := s.db.CreateUser(ctx, store.CreateUserParams{
		Email: normalizeEmail(email), Name: name, Role: store.UserRoleAdmin, PasswordHash: &hash,
	})
	if postgres.IsUniqueViolation(err) {
		return User{}, fmt.Errorf("a user with email %s already exists", email)
	}
	return toUser(u), err
}

func (s *Service) keepAnAdmin(ctx context.Context, q *store.Queries) error {
	n, err := q.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return httpx.Conflict("this is the last admin; make someone else an admin first")
	}
	return nil
}

// issueToken stores a one-time token's hash and returns the token.
func (s *Service) issueToken(ctx context.Context, q *store.Queries, userID uuid.UUID, purpose store.TokenPurpose, ttl time.Duration, by *uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	_, err := q.CreateUserToken(ctx, store.CreateUserTokenParams{
		UserID: userID, Purpose: purpose, TokenHash: sum[:], ExpiresAt: time.Now().Add(ttl), CreatedBy: by,
	})
	return token, err
}

// useToken consumes a one-time token and sets the user's password.
func (s *Service) useToken(ctx context.Context, token string, purpose store.TokenPurpose, password string) (uuid.UUID, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	sum := sha256.Sum256([]byte(token))
	var userID uuid.UUID
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		userID, err = q.ConsumeUserToken(ctx, store.ConsumeUserTokenParams{TokenHash: sum[:], Purpose: purpose})
		if postgres.IsNotFound(err) {
			return zinc.Gone("this link has expired or was already used")
		}
		if err != nil {
			return err
		}
		return q.SetUserPassword(ctx, store.SetUserPasswordParams{ID: userID, PasswordHash: &hash})
	})
	s.forget(ctx, userID)
	return userID, err
}

// send emails in the background: a slow mail server shouldn't slow the
// request, and the link can be sent again.
func (s *Service) send(to, subject, body string) {
	go func() {
		if err := s.mail.Send(to, subject, body); err != nil {
			slog.Error("send email", "to", to, "err", err)
		}
	}()
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
