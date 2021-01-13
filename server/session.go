package main

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

const SessionCookie = "SESSION_ID"

func SetSessionCookie(c *gin.Context, sessionID string) {
	c.SetCookie(
		SessionCookie,
		url.PathEscape(NewSignature("cookie", sessionID)),
		0,
		"/",
		"",
		true,
		true,
	)
}

func Authenticated(c *gin.Context) {
	user := GetUserFromSession(c)
	if user == nil {
		c.Redirect(http.StatusSeeOther, "/")
		c.Abort()
		return
	}
	c.Set("user", user)
}

func GetUserFromSession(c *gin.Context) *User {
	cookie, err := c.Cookie(SessionCookie)
	if err != nil {
		return nil
	}

	signed, err := url.PathUnescape(cookie)
	if err != nil {
		return nil
	}

	name, sessionID, ok := ValidSignature(signed)
	if !ok || name != "cookie" {
		return nil
	}

	user := new(User)
	db.Preload("Sessions").First(user, User{Session: Session{ID: sessionID}})
	if user.ID == "" || user.Session.ID == "" {
		return nil
	}

	return user
}
