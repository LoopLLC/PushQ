package pushq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

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
// It is also called from the admin console's API functions to check auth,
// even though they aren't actually pages.
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
	p.SiteName = "Loop PushQ" // TODO - Config

	return true
}

func pageFail(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "%s\r\n", message)
}

// QStat is a view model for queue/URL stats and config
type QStat struct {
	// Name is the queue name or the URL
	Name        string
	Total       int64
	Today       int64
	ErrToday    int64
	AvgMS       float32
	LogsEnabled bool
	Active      bool
	UpdatedOn   time.Time
}

// QStatKind is the name of the datastore table for queue stats
const QStatKind string = "QStat"

// AdminPage is a view model for the admin page
type AdminPage struct {
	Page
	NumEnq      int64
	NumEnqToday int64
	NumErrToday int64
	Qs          []*QStat
	URLs        []*QStat
}

// admin renders the administrative interface for the server
func admin(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "admin called")

	p := AdminPage{}

	if !initPage(ctx, w, r, &p.Page) {
		return
	}

	p.Title = "Loop PushQ Admin Console"
	now := time.Now().UTC()
	nowf := getTodayf(now)

	log.Debugf(ctx, "admin nowf: %s", nowf)

	// Overall Stats
	var c int64
	var err error
	if c, err = Count(ctx, EnqCt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.NumEnq = c

	if c, err = Count(ctx, EnqCt+nowf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.NumEnqToday = c

	if c, err = Count(ctx, ErrCt+nowf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.NumErrToday = c

	// Queue Stats
	qNames := *QNames
	for qn := range qNames {
		s := QStat{}
		s.Name = qn
		getStats(ctx, &s, s.Name, nowf)
		p.Qs = append(p.Qs, &s)
	}

	q := datastore.NewQuery(AllURLsKind)
	var urls []AllURLs
	if _, err := q.GetAll(ctx, &urls); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, url := range urls {
		s := QStat{}
		s.Name = url.URL
		if err = getStats(ctx, &s, s.Name, nowf); err != nil {
			pageFail(w, err.Error())
			return
		}
		p.URLs = append(p.URLs, &s)
	}

	renderPage(w, r, p, "admin.html")
}

func getOrCreateQStat(ctx context.Context, s *QStat, name string) error {
	var err error
	key := datastore.NewKey(ctx, QStatKind, name, 0, nil)
	err = datastore.Get(ctx, key, s)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			s.Active = true
			s.LogsEnabled = false
			s.Name = name
			if _, err := datastore.Put(ctx, key, s); err != nil {
				return err
			}
		} else if isErrFieldMismatch(err) {
			// Ignore?
		} else {
			return err
		}
	}
	return nil
}

func getStats(ctx context.Context, s *QStat, name string, nowf string) error {

	var c int64
	var err error

	// First look for the latest stored copy of stats, which also
	// has config entries.
	if err = getOrCreateQStat(ctx, s, name); err != nil {
		return err
	}

	if c, err = Count(ctx, EnqCt+name); err == nil {
		s.Total = c
	}
	if c, err = Count(ctx, EnqCt+name+nowf); err == nil {
		s.Today = c
	}
	if c, err = Count(ctx, ErrCt+name+nowf); err == nil {
		s.ErrToday = c
	}
	if c, err = Count(ctx, AvgTotalCt+name+nowf); err == nil {
		if accum, err := Count(ctx, AvgAccumCt+s.Name+nowf); err == nil {
			if c > 0 {
				s.AvgMS = float32(accum) / float32(c)
			}
		}
	}

	s.UpdatedOn = time.Now().UTC()

	// Now re-save the latest stats, retaining config entries
	key := datastore.NewKey(ctx, QStatKind, name, 0, nil)
	if _, err := datastore.Put(ctx, key, s); err != nil {
		return err
	}

	return nil
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

	var p Page
	if !initPage(ctx, w, r, &p) {
		return
	}

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

	var p Page
	if !initPage(ctx, w, r, &p) {
		return
	}

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

// toggleQueueLogs changes the log setting for a single queue
func toggleQueueLogs(w http.ResponseWriter, r *http.Request) {
	var err error

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "toggleQueueLogs called")

	var p Page
	if !initPage(ctx, w, r, &p) {
		return
	}

	// Decode the POST body
	decoder := json.NewDecoder(r.Body)
	var s QStat
	err = decoder.Decode(&s)
	if err != nil {
		failJSON(w, err.Error())
		return
	}

	// Get the currently stored config
	var stored QStat
	key := datastore.NewKey(ctx, QStatKind, s.Name, 0, nil)
	err = datastore.Get(ctx, key, &stored)
	if err != nil {
		if isErrFieldMismatch(err) {
			// Ignore
		} else {
			// It should be there
			failJSON(w, err.Error())
			return
		}
	}

	stored.LogsEnabled = s.LogsEnabled
	_, err = datastore.Put(ctx, key, &stored)
	if err != nil {
		failJSON(w, err.Error())
		return
	}

	okJSON(w, "Ok")
}

// LogPage is a view model for the page displaying queue logs
type LogPage struct {
	Page
	QueueName string
	Logs      []TaskLog
}

func logs(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "logs called")

	p := LogPage{}

	if !initPage(ctx, w, r, &p.Page) {
		return
	}

	params := mux.Vars(r)
	p.QueueName = params["name"]

	p.Title = fmt.Sprintf("Loop PushQ Admin Console - %s Logs", p.QueueName)

	// TODO
	q := datastore.NewQuery(TaskLogKind).Limit(100)
	if _, err := q.GetAll(ctx, &p.Logs); err != nil {
		pageFail(w, err.Error())
		return
	}

	renderPage(w, r, p, "logs.html")
}
