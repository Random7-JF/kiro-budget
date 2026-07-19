package http

import (
	"errors"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/auth"
	"github.com/budget-tracker/budget-tracker/internal/store"
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

// handleRegisterForm serves the account-creation page.
func (s *Server) handleRegisterForm(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	// Already logged in — bounce to the app.
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		if _, err := s.repo.SessionUser(r.Context(), cookie.Value); err == nil {
			stdhttp.Redirect(w, r, "/", stdhttp.StatusSeeOther)
			return
		}
	}
	s.render(w, stdhttp.StatusOK, "register", web.RegisterView{})
}

// handleRegister validates and creates a new account, then logs the user in.
func (s *Server) handleRegister(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	firstName := strings.TrimSpace(r.PostFormValue("first_name"))
	lastName := strings.TrimSpace(r.PostFormValue("last_name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	username := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")

	// Re-render the form with the supplied values on validation failure.
	rerenderWith := func(msg string) {
		s.render(w, stdhttp.StatusUnprocessableEntity, "register", web.RegisterView{
			Error:     msg,
			FirstName: firstName,
			LastName:  lastName,
			Email:     email,
			Username:  username,
		})
	}

	if firstName == "" || lastName == "" {
		rerenderWith("First name and last name are required.")
		return
	}
	if email == "" {
		rerenderWith("Email address is required.")
		return
	}
	if len(username) < 2 {
		rerenderWith("Username must be at least 2 characters.")
		return
	}
	if len(password) < 8 {
		rerenderWith("Password must be at least 8 characters.")
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "Could not process your request. Please try again.")
		return
	}

	user, err := s.repo.CreateUser(r.Context(), username, hash, firstName, lastName, email)
	if err != nil {
		if errors.Is(err, store.ErrUserExists) {
			rerenderWith("That username is already taken. Please choose another.")
			return
		}
		s.renderError(w, stdhttp.StatusInternalServerError, "Could not create account. Please try again.")
		return
	}

	// Log the new user in immediately.
	token, err := auth.NewToken()
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "Account created but could not start a session. Please sign in.")
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.repo.CreateSession(r.Context(), token, user.ID, expires); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "Account created but could not start a session. Please sign in.")
		return
	}
	setSessionCookie(w, token, expires)
	stdhttp.Redirect(w, r, "/", stdhttp.StatusSeeOther)
}
