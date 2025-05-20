package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
	_ "github.com/lib/pq"
)

var (
	fbOauthConfig = &oauth2.Config{
		ClientID:     "1631544887333450",
		ClientSecret: "c4699d2496c5dcaccc9557ff27079433",
		RedirectURL:  "http://localhost:8080/auth/callback",
		Scopes: []string{
			"pages_read_engagement",
			"instagram_content_publish",
			"business_management",
			"pages_show_list",
			"instagram_basic",
		},
		Endpoint: facebook.Endpoint,
	}

	db *sql.DB
)

func main() {
	var err error
	db, err = sql.Open("postgres", "user=postgres password=chetan dbname=instagram_reel_db sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()

	e.GET("/auth/login", loginHandler)
	e.GET("/auth/callback", callbackHandler)

	e.Logger.Fatal(e.Start(":8080"))
}

func loginHandler(c echo.Context) error {
	url := fbOauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func callbackHandler(c echo.Context) error {
	code := c.QueryParam("code")
	token, err := fbOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Failed to exchange token: %v", err))
	}

	resp, err := http.Get("https://graph.facebook.com/v22.0/me?access_token=" + token.AccessToken)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching FB user: %v", err))
	}
	defer resp.Body.Close()

	var user struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to decode FB user: %v", err))
	}

	userID := user.ID

	_, err = db.Exec("INSERT INTO users (fb_user_id, token) VALUES ($1, $2)", userID, token.AccessToken)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("DB insert error (users): %v", err))
	}

	pagesResp, err := http.Get("https://graph.facebook.com/v22.0/me/accounts?fields=id,name,instagram_business_account&access_token=" + token.AccessToken)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching pages: %v", err))
	}
	defer pagesResp.Body.Close()

	var pagesData struct {
		Data []struct {
			ID                       string `json:"id"`
			Name                     string `json:"name"`
			InstagramBusinessAccount struct {
				ID string `json:"id"`
			} `json:"instagram_business_account"`
		} `json:"data"`
	}

	if err := json.NewDecoder(pagesResp.Body).Decode(&pagesData); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to decode pages data: %v", err))
	}

	for _, page := range pagesData.Data {
		instaID := page.InstagramBusinessAccount.ID
		username := ""
		isInstaConnected := false

		if instaID != "" {
			isInstaConnected = true

			igResp, err := http.Get("https://graph.facebook.com/v22.0/" + instaID + "?fields=username&access_token=" + token.AccessToken)
			if err == nil {
				defer igResp.Body.Close()
				var igData struct {
					Username string `json:"username"`
				}
				if err := json.NewDecoder(igResp.Body).Decode(&igData); err == nil {
					username = igData.Username
				}
			}
		}

		_, err = db.Exec(
			"INSERT INTO pages (user_id, page_id, instagram_user_id, instagram_username, isinstaconnected) VALUES ($1, $2, $3, $4, $5)",
			userID, page.ID, instaID, username, isInstaConnected,
		)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("DB insert error (pages): %v", err))
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"facebook_user_id": userID,
		"message":          "User and page data saved successfully!",
	})
}

