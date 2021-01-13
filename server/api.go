package main

import (
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	oauthSpotify "golang.org/x/oauth2/spotify"
)

func RegisterAPI(r gin.IRouter) {
	r.GET("/login", Login)
	r.GET("/spotify_callback", SpotifyCallback)
	r.GET("/token", GetToken)
}

func OauthClient(scopes ...string) *oauth2.Config {
	if len(scopes) == 0 {
		scopes = append(scopes, "user-read-private")
	}
	return &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.ClientRedirectURL,
		Endpoint:     oauthSpotify.Endpoint,
		Scopes:       scopes,
	}
}

func GetToken(c *gin.Context) {
	tokenID, apiKey, ok := c.Request.BasicAuth()
	if !ok {
		c.String(http.StatusUnauthorized, "request must have basic auth")
		c.Abort()
		return
	}

	apiKeyBytes, err := base64.StdEncoding.DecodeString(apiKey)
	if err != nil {
		c.Error(err)
		c.String(http.StatusUnauthorized, "invalid auth")
		c.Abort()
		return
	}

	token := new(Token)
	db.First(token, Token{ID: tokenID})
	if token.ID == "" {
		c.String(http.StatusUnauthorized, "invalid auth")
		c.Abort()
		return
	}

	if err := bcrypt.CompareHashAndPassword(token.APIKeyHash, apiKeyBytes); err != nil {
		c.Error(err)
		c.String(http.StatusUnauthorized, "invalid auth")
		c.Abort()
		return
	}

	oauthToken := token.IntoOauth()
	c.String(http.StatusOK, oauthToken.Type()+" "+oauthToken.AccessToken)
}

func Login(c *gin.Context) {
	state := NewSignature("login", RandomString())
	c.Redirect(http.StatusSeeOther, OauthClient().AuthCodeURL(state))
}

func PostLogin(c *gin.Context, code, sessionID string) {
	token, err := OauthClient().Exchange(c, code)
	if err != nil {
		c.String(http.StatusUnauthorized, "Could not complete authorization: invalid code")
		c.Abort()
		return
	}

	id, err := GetSpotifyID(c, token)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		c.Abort()
		return
	}

	user := new(User)
	db.FirstOrCreate(user, User{ID: id})

	db.Delete(Session{
		UserID: user.ID,
	})
	db.Create(&Session{
		ID:     sessionID,
		UserID: user.ID,

		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expires:      token.Expiry,
	})

	SetSessionCookie(c, sessionID)

	c.HTML(http.StatusOK, "redirect.html", gin.H{
		"path": "/home",
		"text": "Complete Login",
	})
}

func CreateToken(c *gin.Context, code, tokenID string) {
	token := new(Token)
	db.Find(token, Token{ID: tokenID})
	if token.ID == "" {
		c.String(http.StatusUnauthorized, "Could not complete authorization: invalid state")
		c.Abort()
		return
	}

	oauthToken, err := OauthClient().Exchange(c, code)
	if err != nil {
		c.String(http.StatusUnauthorized, "Could not complete authorization: invalid code")
		c.Error(err)
		c.Abort()
		return
	}

	apiKey := RandomString()
	hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), config.HashCost)
	if err != nil {
		c.String(http.StatusUnauthorized, "Could not complete authorization: something went wrong creating the token")
		c.Error(err)
		c.Abort()
		return
	}

	token.APIKeyHash = hash
	token.FromOauth(oauthToken)
	db.Save(token)

	c.HTML(http.StatusOK, "token.html", gin.H{
		"Name":   token.Name,
		"ID":     token.ID,
		"APIKey": apiKey,
		"Scopes": token.Scopes,
	})
}

func SpotifyCallback(c *gin.Context) {
	if err := c.Query("error"); err != "" {
		c.String(http.StatusBadRequest, "Could not complete authorization: %s", err)
		c.Abort()
		return
	}

	code := c.Query("code")
	if code == "" {
		c.String(http.StatusBadRequest, "Could not complete authorization: no auth code present")
		c.Abort()
		return
	}

	name, value, ok := ValidSignature(c.Query("state"))
	if !ok {
		c.String(http.StatusUnauthorized, "Could not complete authorization: invalid state")
		c.Abort()
		return
	}

	switch name {
	case "login":
		PostLogin(c, code, value)
	case "token":
		CreateToken(c, code, value)
	default:
		c.String(http.StatusUnauthorized, "Could not complete authorization: invalid state")
		c.Abort()
		return
	}
}
