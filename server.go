package pushq

// This file is the entry point to the web application.  It has routing
// and the REST API functions for enqueuing tasks.

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"math/rand"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"

	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/taskqueue"
	"google.golang.org/appengine/urlfetch"
)

// TaskHeader is an HTTP header that gets added to headers for the target URL
type TaskHeader struct {
	Name  string `datastore:"n" json:"name"`
	Value string `datastore:"v" json:"value"`
}

// Task is a model for what callers need to POST to enqueue a task
type Task struct {
	URL            string       `datastore:"u" json:"url"`
	DelaySeconds   int          `datastore:"d" json:"delaySeconds"`
	Payload        string       `datastore:"p" json:"payload"`
	QueueName      string       `datastore:"q" json:"queueName"`
	Headers        []TaskHeader `datastore:"h" json:"headers"`
	TimeoutSeconds int          `datastore:"t" json:"timeoutSeconds"`
}

// Cache templates
var templates = template.Must(template.ParseFiles("tmpl/admin.html",
	"tmpl/header.html", "tmpl/footer.html", "tmpl/keys.html"))

// init initializes the web application by configuring routes
func init() {

	// Static Routes
	http.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.Dir("./static"))))

	// Gorilla mux for REST API routes and admin console
	muxRouter := mux.NewRouter()

	// Admin pages and JSON endpoints
	muxRouter.HandleFunc("/admin", admin).Methods("GET")
	muxRouter.HandleFunc("/admin/keys", keys).Methods("GET")
	muxRouter.HandleFunc("/admin/newapikey", newAPIKey).Methods("GET")
	muxRouter.HandleFunc("/admin/delapikey", delAPIKey).Methods("POST")

	// REST API
	muxRouter.HandleFunc("/enq", enq).Methods("POST")
	muxRouter.HandleFunc("/callback", callback).Methods("POST")
	muxRouter.HandleFunc("/test", test).Methods("POST")
	muxRouter.HandleFunc("/counts", getAllCounts).Methods("GET")

	http.Handle("/", muxRouter)
}

// isErrFieldMismatch checks datastore errors for model mismatch
func isErrFieldMismatch(err error) bool {
	_, ok := err.(*datastore.ErrFieldMismatch)
	return ok
}

// XAPIKEY is the HTTP Header for the API Key
const XAPIKEY string = "X-Loop-APIKey"

// XAPISECRET is the HTTP Header for the API Secret
const XAPISECRET string = "X-Loop-APISecret"

// ISO8601 is the standard date format string
const ISO8601 string = "2006-01-02T15:04:00Z"

// ISO8601D is the standard date format without time
const ISO8601D string = "2006-01-02"

// APIKeyKind is the name of the datastore Kind (table) for keys
const APIKeyKind string = "APIKey"

// APIKey is a record that stores the secret hash and other information
// about the API account to manage access to the REST API.
type APIKey struct {
	Key        string
	Secret     string `datastore:"-"`
	SecretHash []byte
}

// auth checks to make sure the caller has rights to use the REST API.
// This is not the same as the admin console auth, which is based on
// Google accounts.  This auth relies on API Keys.
func auth(ctx context.Context, r *http.Request) bool {

	//log.Debugf(ctx, "auth request: %+v", r)

	key := r.Header.Get(XAPIKEY)
	if key == "" {
		log.Debugf(ctx, "%s missing", XAPIKEY)
		return false
	}

	secret := r.Header.Get(XAPISECRET)
	if key == "" {
		log.Debugf(ctx, "%s missing", XAPISECRET)
		return false
	}

	k := datastore.NewKey(ctx, APIKeyKind, key, 0, nil)
	apiKey := APIKey{}
	if err := datastore.Get(ctx, k, &apiKey); err != nil {
		log.Debugf(ctx, "Error retrieving %s: %s", APIKeyKind, err.Error())
		return false
	}

	err := bcrypt.CompareHashAndPassword(apiKey.SecretHash, []byte(secret))

	return err == nil
}

// genKeySecret auto-generates an API Key and Secret, and the bcrypt Hash
// to be stored for later comparison during authentication
func genKeySecret(apiKey *APIKey) error {
	rand.Seed(time.Now().UnixNano())
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	key := make([]byte, 16)
	for i := range key {
		key[i] = letters[rand.Intn(len(letters))]
	}
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = letters[rand.Intn(len(letters))]
	}

	apiKey.Key = string(key)
	apiKey.Secret = string(secret)

	password := []byte(secret)

	h, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	apiKey.SecretHash = h

	return nil
}

func incrementCounters(ctx context.Context, name string, now time.Time) {

	countAllName := name + "All"

	// All
	if err := Increment(ctx, countAllName); err != nil {
		log.Errorf(ctx, err.Error())
	}

	localt := now
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		localt = now.In(loc)
	}
	countTodayName := countAllName + localt.Format(ISO8601D)

	// Today
	if err := Increment(ctx, countTodayName); err != nil {
		log.Errorf(ctx, err.Error())
	}
}

// enq enqueues a task
func enq(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "enq called")

	if !auth(ctx, r) {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	var task Task
	var jsonb []byte
	jsonb, _ = ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(jsonb, &task); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	qNames := map[string]bool{
		"default":     true,
		"crm":         true,
		"campaigns":   true,
		"integration": true,
		"reports":     true,
		"messaging":   true,
	}

	if !qNames[task.QueueName] {
		http.Error(w, "Invalid QueueName", http.StatusNotAcceptable)
		return
	}

	// Create the task
	t := taskqueue.Task{}
	t.Path = "/callback"
	t.Delay = time.Duration(task.DelaySeconds) * time.Second
	t.Payload = jsonb // Use the entire submitted task as the payload

	// Enqueue the task
	if _, err := taskqueue.Add(ctx, &t, task.QueueName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	incrementCounters(ctx, "Enqueue", time.Now().UTC())
}

// callback POSTs the task payload to the URL.
func callback(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "callback called")

	// Look for one of Google's headers: X-AppEngine-QueueName
	// These headers are removed if an external caller sets them,
	// so they can be used to make sure the request is valid.
	xq := r.Header.Get("X-AppEngine-QueueName")
	if xq == "" {
		http.Error(w, "Missing required header", 400)
		return
	}

	// Unmarshal the task
	var task Task
	var jsonb []byte
	jsonb, _ = ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(jsonb, &task); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	log.Debugf(ctx, "callback payload: %+v", task)

	// Double check the queue name
	if xq != task.QueueName {
		http.Error(w, "header QueueName mismatch", 400)
		return
	}

	// Initialize the http client
	var client = urlfetch.Client(ctx)
	client.Timeout = time.Duration(task.TimeoutSeconds) * time.Second

	req, err := http.NewRequest("POST", task.URL, bytes.NewBuffer(jsonb))
	if err != nil {
		log.Debugf(ctx, "Unable to create callback request: %s", err.Error())
		http.Error(w, "Callback Failed", 400)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Add custom task headers
	for _, h := range task.Headers {
		req.Header.Set(h.Name, h.Value)
	}

	// Make the request
	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		log.Debugf(ctx, "Callback client failed: %s", err.Error())
		http.Error(w, "Callback Failed", 400)
		return
	} else if resp.StatusCode != http.StatusOK {
		log.Debugf(ctx, "Callback failed: %s", resp.Status)
		http.Error(w, "Callback Failed", 400)
		return
	}

	log.Debugf(ctx, "callback got resp: %+v", resp)
}

func test(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "test called")

}

// CounterTotal is used to pass totals back as JSON from getAllCounts
type CounterTotal struct {
	Name  string
	Total int
}

func getAllCounts(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	counterNames, err := GetAllCounterNames(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var totals []CounterTotal

	for _, counterName := range counterNames {
		total := CounterTotal{Name: counterName}
		var c int
		var err error
		if c, err = Count(ctx, counterName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total.Total = c
		totals = append(totals, total)
	}

	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	enc.Encode(totals)
}
