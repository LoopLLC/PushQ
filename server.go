package pushq

// This file is the entry point to the web application.  It has routing
// and the REST API functions for enqueuing tasks.

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"math/rand"
	"strings"

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
	Payload        string       `datastore:"p,noindex" json:"payload"`
	QueueName      string       `datastore:"q" json:"queueName"`
	Headers        []TaskHeader `datastore:"h,noindex" json:"headers"`
	TimeoutSeconds int          `datastore:"t" json:"timeoutSeconds"`
}

// TaskLog is a model for log entries about tasks
type TaskLog struct {
	Task
	LogType string    `datastore:"lty" json:"logType"`
	UTC     time.Time `datastore:"utc" json:"enqUTC"`
	Code    int       `datastore:"cd" json:"code"`
	Message string    `datastore:"msg" json:"message"`
}

// TaskLogKind is the name of the TaskLog table
const TaskLogKind string = "TaskLog"

// QNames is the list of queues, which should match queue.yaml
var QNames *map[string]bool

var templates *template.Template

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
	muxRouter.HandleFunc("/admin/logs/{name}", logs).Methods("GET")
	muxRouter.HandleFunc("/admin/newapikey", newAPIKey).Methods("GET")
	muxRouter.HandleFunc("/admin/delapikey", delAPIKey).Methods("POST")
	muxRouter.HandleFunc("/admin/toggleQueueLogs",
		toggleQueueLogs).Methods("POST")

	// REST API
	muxRouter.HandleFunc("/enq", enq).Methods("POST")
	muxRouter.HandleFunc("/callback", callback).Methods("POST")
	muxRouter.HandleFunc("/test", test).Methods("POST")
	muxRouter.HandleFunc("/testerr", testerr).Methods("POST")
	muxRouter.HandleFunc("/counts", getAllCounts).Methods("GET")

	// Make sure this matches queue.yaml
	// These also end up getting entries in the QStat table
	qNames := map[string]bool{
		"default":     true,
		"crm":         true,
		"campaigns":   true,
		"integration": true,
		"reports":     true,
		"messaging":   true,
	}

	QNames = &qNames

	funcMap := template.FuncMap{
		"fmtms":  fmtms,
		"fmtutc": fmtutc,
	}

	// Cache templates
	templates = template.Must(
		template.New("all").Funcs(funcMap).ParseFiles("tmpl/admin.html",
			"tmpl/header.html", "tmpl/footer.html", "tmpl/keys.html",
			"tmpl/logs.html"))

	http.Handle("/", muxRouter)
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

// AllURLsKind is the name of the datastore Kind for URLs
const AllURLsKind string = "AllURLs"

// AllURLs is used to store a record of all URLs used as the callback for tasks
type AllURLs struct {
	URL string
}

// APIKey is a record that stores the secret hash and other information
// about the API account to manage access to the REST API.
// (This isn't actually a cryptographic key, it's just a username/password)
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

func incrementCounters(ctx context.Context,
	name string, now time.Time, by int64) {

	countAllName := name

	// All
	if err := Increment(ctx, countAllName, by); err != nil {
		log.Errorf(ctx, err.Error())
	}

	countTodayName := countAllName + getTodayf(now)

	// Today
	if err := Increment(ctx, countTodayName, by); err != nil {
		log.Errorf(ctx, err.Error())
	}
}

// saveLog saves a record to datastore with task info.
func saveLog(
	ctx context.Context,
	task *Task,
	logType string,
	code int,
	message string,
) {

	var tl TaskLog
	tl.Task = *task
	tl.LogType = logType
	tl.Code = code
	tl.Message = message
	tl.UTC = time.Now().UTC()

	key := datastore.NewIncompleteKey(ctx, TaskLogKind, nil)
	if _, err := datastore.Put(ctx, key, &tl); err != nil {
		log.Debugf(ctx, err.Error())
	}
}

// enq enqueues a task
func enq(w http.ResponseWriter, r *http.Request) {
	var err error

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "enq called")

	if !auth(ctx, r) {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	var task Task
	var jsonb []byte
	jsonb, _ = ioutil.ReadAll(r.Body)
	if err = json.Unmarshal(jsonb, &task); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	qNames := *QNames
	if !qNames[task.QueueName] {
		http.Error(w, "Invalid QueueName", http.StatusNotAcceptable)
		return
	}

	// Get the Queue config
	var s QStat
	if err = getOrCreateQStat(ctx, &s, task.QueueName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create the task
	t := taskqueue.Task{}
	t.Path = "/callback"
	t.Delay = time.Duration(task.DelaySeconds) * time.Second
	t.Payload = jsonb // Use the entire submitted task as the payload

	// If we are testing errors, only retry once
	if strings.HasSuffix(task.URL, "testerr") {
		ro := taskqueue.RetryOptions{}
		ro.RetryLimit = 1
		t.RetryOptions = &ro
	}

	// Enqueue the task
	if _, err := taskqueue.Add(ctx, &t, task.QueueName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		incrementCounters(ctx, "EnqueueError", time.Now().UTC(), 1)

		if s.LogsEnabled {
			saveLog(ctx, &task, "EnqueueError", 0, err.Error())
		}

		return
	}

	if s.LogsEnabled {
		saveLog(ctx, &task, "Enqueue", 0, "")
	}

	nowutc := time.Now().UTC()

	incrementCounters(ctx, EnqCt, nowutc, 1)
	incrementCounters(ctx, EnqCt+task.QueueName, nowutc, 1)
	incrementCounters(ctx, EnqCt+task.URL, nowutc, 1)
	recordURL(ctx, task.URL)
}

// recordURL saves the URL so that we can get a list of all unique URLs
// used as the callback for enqueued tasks.
func recordURL(ctx context.Context, url string) {
	k := datastore.NewKey(ctx, AllURLsKind, url, 0, nil)
	apiKey := APIKey{}
	if err := datastore.Get(ctx, k, &apiKey); err != nil {
		if err == datastore.ErrNoSuchEntity {
			u := AllURLs{URL: url}
			if _, err = datastore.Put(ctx, k, &u); err != nil {
				log.Debugf(ctx, "Unable to record AlURLs: %s", url)
			}
		}
	}
}

// callback POSTs the task payload to the URL.
func callback(w http.ResponseWriter, r *http.Request) {
	var err error

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

	// Get the Queue config
	var s QStat
	if err = getOrCreateQStat(ctx, &s, task.QueueName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Initialize the http client
	var client = urlfetch.Client(ctx)
	client.Timeout = time.Duration(task.TimeoutSeconds) * time.Second

	req, err := http.NewRequest("POST", task.URL, bytes.NewBuffer(jsonb))
	if err != nil {
		log.Debugf(ctx, "Unable to create callback request: %s", err.Error())

		if s.LogsEnabled {
			saveLog(ctx, &task, "NewRequestError", 0, err.Error())
		}

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
	before := time.Now().UTC()
	if resp, err = client.Do(req); err != nil {
		log.Debugf(ctx, "Callback client failed: %s", err.Error())

		if s.LogsEnabled {
			saveLog(ctx, &task, "ClientError", 0, err.Error())
		}

		http.Error(w, "Callback Failed", 400)
		return
	} else if resp.StatusCode != http.StatusOK {
		log.Debugf(ctx, "Callback Failed: %s", resp.Status)

		if s.LogsEnabled {
			saveLog(ctx, &task, "CallbackError",
				resp.StatusCode, resp.Status)
		}

		nowutc := time.Now().UTC()
		incrementCounters(ctx, ErrCt, nowutc, 1)
		incrementCounters(ctx, ErrCt+task.URL, nowutc, 1)
		incrementCounters(ctx, ErrCt+task.QueueName, nowutc, 1)
		http.Error(w, "Callback Failed", 400)
		return
	}

	// Elapsed time
	after := time.Now().UTC()
	diff := after.Sub(before)
	elapsedNs := diff.Nanoseconds()
	ms := elapsedNs / int64(1000000)

	// Store elapsed time for average calculations
	nowutc := time.Now().UTC()
	incrementCounters(ctx, AvgTotalCt+task.URL, nowutc, 1)
	incrementCounters(ctx, AvgAccumCt+task.URL, nowutc, ms)
	incrementCounters(ctx, AvgTotalCt+task.QueueName, nowutc, 1)
	incrementCounters(ctx, AvgAccumCt+task.QueueName, nowutc, ms)

	log.Debugf(ctx, "callback got resp in %dns: %+v", elapsedNs, resp)

	if s.LogsEnabled {
		saveLog(ctx, &task, "CallbackSuccess",
			resp.StatusCode, resp.Status)
	}
}

func test(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "test called")

}

func testerr(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	log.Debugf(ctx, "testerr called")

	http.Error(w, "testerr", 400)
}

// CounterTotal is used to pass totals back as JSON from getAllCounts
type CounterTotal struct {
	Name  string
	Total int64
}

func getAllCounts(w http.ResponseWriter, r *http.Request) {

	ctx := appengine.NewContext(r)

	if !auth(ctx, r) {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	counterNames, err := GetAllCounterNames(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var totals []CounterTotal

	for _, counterName := range counterNames {
		total := CounterTotal{Name: counterName}
		var c int64
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
