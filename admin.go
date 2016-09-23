package pushq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"
)

// This file contains the admin console portion of the web server

// Page is a model passed into html templates
type Page struct {
	SiteName   string
	Title      string
	Name       string
	IsLoggedIn bool
	AuthURL    string
	UserID     string
}

// KeysPage is a view model for the API Keys page
type KeysPage struct {
	Page
	Keys []APIKey
}

// renderPage renders a standard header-body-footer page
func renderPage(w http.ResponseWriter, r *http.Request, model interface{},
	bodyTemplate string) {

	buf := &bytes.Buffer{}

	tnames := [3]string{"header.html", bodyTemplate, "footer.html"}
	for i := 0; i < 3; i++ {
		err := templates.ExecuteTemplate(buf, tnames[i], model)
		if err != nil {
			// Prints the error to the browser
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// initPage checks auth and returns false if the page should not be rendered.
// This auth is for the admin console, not for the REST API.
func initPage(ctx context.Context,
	w http.ResponseWriter, r *http.Request, p *Page) bool {

	// Get the Google user account
	u := user.Current(ctx)
	if u == nil {
		p.IsLoggedIn = false
		p.AuthURL, _ = user.LoginURL(ctx, "/admin")
	} else {
		p.IsLoggedIn = true
		p.UserID = u.ID
		p.AuthURL, _ = user.LogoutURL(ctx, "/admin")
	}

	// Make sure we are logged in
	if !p.IsLoggedIn {
		// Redirect to the Google signin page
		http.Redirect(w, r, p.AuthURL, 302)
		return false
	}

	// Only allow admins
	if !user.IsAdmin(ctx) {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return false
	}

	p.Name = u.String()
	p.SiteName = "Loop PushQ"

	return true
}

func pageFail(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "%s\r\n", message)
}

// APIResponse is serialized to json for success and some error responses
type APIResponse struct {
	OK      bool        `json:"ok"`
	Message string      `json:"msg"`
	Data    interface{} `json:"data"`
}

// okJSON writes a success response
func okJSON(w http.ResponseWriter, data interface{}) {
	r := APIResponse{OK: true, Message: "OK", Data: data}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(r)
}

// failJSON writes a failure response
func failJSON(w http.ResponseWriter, message string) {
	r := APIResponse{OK: false, Message: message, Data: message}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(r)
}

// admin renders the administrative interface for the server
func admin(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "admin called")

	p := Page{}

	if !initPage(ctx, w, r, &p) {
		return
	}

	p.Title = "Loop PushQ Admin Console"

	renderPage(w, r, p, "admin.html")
}

// keys renders the API Keys admin page
func keys(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "keys called")

	p := KeysPage{}

	if !initPage(ctx, w, r, &p.Page) {
		return
	}

	q := datastore.NewQuery(APIKeyKind)
	if _, err := q.GetAll(ctx, &p.Keys); err != nil {
		pageFail(w, err.Error())
		return
	}

	p.Title = "Loop PushQ Admin Console - Keys"

	renderPage(w, r, p, "keys.html")
}

// newAPIKey is called from JS on the keys page.  It creates and
// emits a new key as JSON
func newAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "newAPIKey called")

	// Generate the key
	ak := APIKey{}
	if err := genKeySecret(&ak); err != nil {
		failJSON(w, err.Error())
	}

	// Save the key
	k := datastore.NewKey(ctx, APIKeyKind, ak.Key, 0, nil)
	if _, err := datastore.Put(ctx, k, &ak); err != nil {
		failJSON(w, err.Error())
	}

	okJSON(w, ak)
}

// delAPIKey is called from JS on the keys page.  It deletes an API Key.
func delAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "delAPIKey called")

	// Decode the POST body
	decoder := json.NewDecoder(r.Body)
	var ak APIKey
	err := decoder.Decode(&ak)
	if err != nil {
		failJSON(w, err.Error())
		return
	}

	// Delete the key
	k := datastore.NewKey(ctx, APIKeyKind, ak.Key, 0, nil)
	if err := datastore.Delete(ctx, k); err != nil {
		failJSON(w, err.Error())
	}

	time.Sleep(500 * time.Millisecond)

	okJSON(w, ak)
}
