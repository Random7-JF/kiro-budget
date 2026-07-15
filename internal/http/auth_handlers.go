package http

import (
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/auth"
	"github.com/budget-tracker/budget-tracker/internal/web"
)

// setSessionCookie writes the session cookie with the given token and expiry.
func setSessionCookie(w stdhttp.ResponseWriter, token string, expires time.Time) {
	stdhttp.SetCookie(w, &stdhttp.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: stdhttp.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie.
func clearSessionCookie(w stdhttp.ResponseWriter) {
	stdhttp.SetCookie(w, &stdhttp.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: stdhttp.SameSiteLaxMode,
	})
}

// handleLoginForm serves the login page. If the request already carries a valid
// session, it redirects to the application root.
func (s *Server) handleLoginForm(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		if _, err := s.repo.SessionUser(r.Context(), cookie.Value); err == nil {
			stdhttp.Redirect(w, r, "/", stdhttp.StatusSeeOther)
			return
		}
	}
	s.renderLogin(w, stdhttp.StatusOK, "")
}

// handleLogin verifies credentials, creates a session, and sets the cookie.
func (s *Server) handleLogin(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")

	user, hash, err := s.repo.UserByUsername(r.Context(), username)
	if err != nil || !auth.CheckPassword(hash, password) {
		s.renderLogin(w, stdhttp.StatusUnauthorized, "Invalid username or password.")
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "Could not start a session. Please try again.")
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.repo.CreateSession(r.Context(), token, user.ID, expires); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "Could not start a session. Please try again.")
		return
	}
	setSessionCookie(w, token, expires)
	stdhttp.Redirect(w, r, "/", stdhttp.StatusSeeOther)
}

// handleLogout deletes the session and clears the cookie.
func (s *Server) handleLogout(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		_ = s.repo.DeleteSession(r.Context(), cookie.Value)
	}
	clearSessionCookie(w)
	stdhttp.Redirect(w, r, "/login", stdhttp.StatusSeeOther)
}

// renderLogin renders the login page with an optional error message.
func (s *Server) renderLogin(w stdhttp.ResponseWriter, status int, errMsg string) {
	s.render(w, status, "login", web.LoginView{Error: errMsg})
}
