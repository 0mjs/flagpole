package identity

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/session"
)

// sessionValue is the key the session cookie stores the session ID under.
const sessionValue = "sid"

type actorKey struct{}

// Authenticate loads the signed-in user, if any, for the rest of the chain.
// It never rejects: RequireRole does that, so public routes can share the
// group.
func (s *Service) Authenticate() zinc.HandlerFunc {
	return func(c *zinc.Context) error {
		sess, ok := session.Get(c)
		if !ok {
			return c.Next()
		}
		if id := sess.Get(sessionValue); id != "" {
			if u, err := s.UserForSession(c.Context(), id); err == nil {
				c.Set(actorKey{}, u)
			} else if err := sess.Delete(sessionValue); err != nil {
				return err
			}
		}
		return c.Next()
	}
}

// Actor returns the signed-in user. Use it in routes behind RequireRole.
func Actor(c *zinc.Context) User {
	return zinc.MustValue[User](c, actorKey{})
}

// RequireRole lets the request through only for a signed-in user whose role
// includes min: 401 when no one is signed in, 403 when the role is too low.
func RequireRole(min Role) zinc.HandlerFunc {
	return func(c *zinc.Context) error {
		u, ok := zinc.Value[User](c, actorKey{})
		if !ok {
			return zinc.Unauthorized("sign in to continue")
		}
		if !u.Role.Allows(min) {
			return zinc.Forbidden("this needs the " + string(min) + " role")
		}
		return c.Next()
	}
}

// Routes registers the sign-in routes on auth and user management on users.
func (s *Service) Routes(auth, users *zinc.Group) {
	auth.Tags("auth")
	auth.Post("/login", zinc.Typed(s.login)).Name("login").
		Summary("Sign in").
		Description("Checks the email and password and starts a session, held in an HttpOnly cookie. Ten attempts per email per 15 minutes.").
		Errors(http.StatusUnauthorized, http.StatusTooManyRequests).
		RequestExample("an admin", loginInput{Email: "ada@example.com", Password: "correct-horse-battery"})
	auth.Post("/logout", zinc.Typed(s.logout)).Name("logout").Summary("Sign out")
	auth.Get("/me", RequireRole(Viewer), zinc.Typed(s.me)).Name("getMe").Summary("The signed-in user").
		Security("session").Errors(http.StatusUnauthorized)
	auth.Post("/invites/accept", zinc.Typed(s.acceptInvite)).Name("acceptInvite").
		Summary("Accept an invite").
		Description("Sets the password for an invited user and signs them in.").
		Errors(http.StatusGone)
	auth.Post("/password-reset", zinc.Typed(s.requestReset)).Name("requestPasswordReset").Status(http.StatusAccepted).
		Summary("Request a password reset").
		Description("Emails a reset link if the account exists. The answer is the same either way.")
	auth.Post("/password-reset/confirm", zinc.Typed(s.confirmReset)).Name("confirmPasswordReset").
		Summary("Set a new password").
		Description("Uses the emailed token, and signs the user out everywhere.").
		Errors(http.StatusGone)

	users.Use(RequireRole(Admin))
	users.Tags("users").Security("session:admin")
	users.Get("", zinc.Typed(s.listUsers)).Name("listUsers").Summary("List users")
	users.Post("", zinc.Typed(s.invite)).Name("inviteUser").Status(http.StatusCreated).
		Summary("Invite a user").
		Description("Creates the user without a password and emails them a link to set one.").
		Errors(http.StatusConflict)
	users.Patch("/{id}", zinc.Typed(s.changeRole)).Name("changeUserRole").Summary("Change a user's role").
		Errors(http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	users.Post("/{id}/disable", zinc.Typed(s.disable)).Name("disableUser").Summary("Disable a user").
		Description("Ends their sessions. The last admin can't be disabled.").
		Errors(http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	users.Post("/{id}/enable", zinc.Typed(s.enable)).Name("enableUser").Summary("Re-enable a user").Errors(http.StatusNotFound)
	users.Delete("/{id}/sessions", zinc.Typed(s.signOutEverywhere)).Name("signOutUser").Summary("Sign a user out everywhere")
}

type loginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (loginInput) OpenAPIName() string { return "Login" }

func (s *Service) login(c *zinc.Context, in loginInput) (User, error) {
	u, id, err := s.Login(c.Context(), in.Email, in.Password, c.IP())
	if err != nil {
		return User{}, err
	}
	return u, session.MustGet(c).Set(sessionValue, id)
}

func (s *Service) logout(c *zinc.Context, _ struct{}) (zinc.NoContent, error) {
	sess := session.MustGet(c)
	if id := sess.Get(sessionValue); id != "" {
		s.Logout(c.Context(), id)
	}
	return zinc.NoContent{}, sess.Delete(sessionValue)
}

func (s *Service) me(c *zinc.Context, _ struct{}) (User, error) {
	return Actor(c), nil
}

type acceptInviteInput struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=12,max=128" doc:"At least 12 characters."`
}

func (acceptInviteInput) OpenAPIName() string { return "AcceptInvite" }

func (s *Service) acceptInvite(c *zinc.Context, in acceptInviteInput) (User, error) {
	u, id, err := s.AcceptInvite(c.Context(), in.Token, in.Password)
	if err != nil {
		return User{}, err
	}
	return u, session.MustGet(c).Set(sessionValue, id)
}

type resetRequestInput struct {
	Email string `json:"email" validate:"required,email"`
}

func (resetRequestInput) OpenAPIName() string { return "PasswordReset" }

func (s *Service) requestReset(c *zinc.Context, in resetRequestInput) (zinc.NoContent, error) {
	return zinc.NoContent{}, s.RequestPasswordReset(c.Context(), in.Email)
}

type resetConfirmInput struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=12,max=128" doc:"At least 12 characters."`
}

func (resetConfirmInput) OpenAPIName() string { return "ConfirmPasswordReset" }

func (s *Service) confirmReset(c *zinc.Context, in resetConfirmInput) (zinc.NoContent, error) {
	return zinc.NoContent{}, s.ResetPassword(c.Context(), in.Token, in.Password)
}

func (s *Service) listUsers(c *zinc.Context, _ struct{}) ([]User, error) {
	return s.ListUsers(c.Context())
}

type inviteInput struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required,max=100"`
	Role  Role   `json:"role" validate:"required"`
}

func (inviteInput) OpenAPIName() string { return "Invite" }

type invited struct {
	Location string `header:"Location" json:"-"`
	User
}

func (invited) OpenAPIName() string { return "InvitedUser" }

func (s *Service) invite(c *zinc.Context, in inviteInput) (invited, error) {
	u, err := s.Invite(c.Context(), Actor(c), in.Email, in.Name, in.Role)
	return invited{Location: "/api/v1/users/" + u.ID.String(), User: u}, err
}

type userID struct {
	ID uuid.UUID `path:"id"`
}

type changeRoleInput struct {
	ID   uuid.UUID `path:"id"`
	Role Role      `json:"role" validate:"required"`
}

func (changeRoleInput) OpenAPIName() string { return "ChangeRole" }

func (s *Service) changeRole(c *zinc.Context, in changeRoleInput) (User, error) {
	return s.ChangeRole(c.Context(), Actor(c), in.ID, in.Role)
}

func (s *Service) disable(c *zinc.Context, in userID) (User, error) {
	return s.SetDisabled(c.Context(), Actor(c), in.ID, true)
}

func (s *Service) enable(c *zinc.Context, in userID) (User, error) {
	return s.SetDisabled(c.Context(), Actor(c), in.ID, false)
}

func (s *Service) signOutEverywhere(c *zinc.Context, in userID) (zinc.NoContent, error) {
	return zinc.NoContent{}, s.SignOutEverywhere(c.Context(), in.ID)
}
