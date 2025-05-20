package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "log"
    "net/http"

    "github.com/gin-gonic/gin"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/facebook"
    _ "github.com/lib/pq"
)

var (
    fbOauthConfig = &oauth2.Config{
        ClientID:     "1631544887333450",
        ClientSecret: "c4699d2496c5dcaccc9557ff27079433",
        RedirectURL:  "http://localhost:8080/auth/callback",
        Scopes:       []string{"pages_read_engagement","instagram_content_publish","business_management", "pages_show_list", "instagram_basic"},
        Endpoint:     facebook.Endpoint,
    }

    db *sql.DB
)

func main() {
    // Connect to Postgres
    var err error
    db, err = sql.Open("postgres", "user=postgres password=chetan dbname=instagram_reel_db sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }

    r := gin.Default()
    r.GET("/auth/login", loginHandler)
    r.GET("/auth/callback", callbackHandler)

    r.Run(":8080")
}

// Redirect to Facebook login
func loginHandler(c *gin.Context) {
    url := fbOauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
    c.Redirect(http.StatusTemporaryRedirect, url)
}

// Handle OAuth callback
func callbackHandler(c *gin.Context) {
    code := c.Query("code")
    token, err := fbOauthConfig.Exchange(context.Background(), code)
    if err != nil {
        c.String(http.StatusBadRequest, "Failed to exchange token: %s", err)
        return
    }

    // Get Facebook User ID
    resp, err := http.Get("https://graph.facebook.com/me?access_token=" + token.AccessToken)
    if err != nil {
        c.String(http.StatusInternalServerError, "Error fetching FB user: %s", err)
        return
    }
    defer resp.Body.Close()

    var user struct {
        ID string `json:"id"`
    }
    json.NewDecoder(resp.Body).Decode(&user)

    // Get Instagram Business ID from linked pages
    pagesResp, err := http.Get("https://graph.facebook.com/v18.0/me/accounts?fields=instagram_business_account&access_token=" + token.AccessToken)
    if err != nil {
        c.String(http.StatusInternalServerError, "Error fetching pages: %s", err)
        return
    }
    defer pagesResp.Body.Close()

    var pagesData struct {
        Data []struct {
            InstagramBusinessAccount struct {
                ID string `json:"id"`
            } `json:"instagram_business_account"`
        } `json:"data"`
    }
    json.NewDecoder(pagesResp.Body).Decode(&pagesData)

    instaID := ""
    if len(pagesData.Data) > 0 && pagesData.Data[0].InstagramBusinessAccount.ID != "" {
        instaID = pagesData.Data[0].InstagramBusinessAccount.ID
    }

    // Save to database
    _, err = db.Exec("INSERT INTO users (fb_user_id, insta_user_id, token) VALUES ($1, $2, $3)", user.ID, instaID, token.AccessToken)
    if err != nil {
        c.String(http.StatusInternalServerError, "DB insert error: %s", err)
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "facebook_user_id": user.ID,
        "instagram_user_id": instaID,
        "Token" : token.AccessToken,
        "message": "User data saved successfully!",
    })
}
