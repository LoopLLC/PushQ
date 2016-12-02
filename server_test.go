package pushq

// First create config.json.
// goapp serve in a separate console window before running tests.
// go test to test localhost
// go test -args [env] to test an environment configured in config.json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"
)

// Environment is a config entry for running tests against local, beta, etc.
type Environment struct {
	EnvName       string
	APIURL        string
	APITestKey    string
	APITestSecret string
}

// Config represents config.json
type Config struct {
	Environments []Environment
}

var config Config
var testEnv Environment

func init() {

	// Read the config file
	fb, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Parse it
	err = json.Unmarshal(fb, &config)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	var localEnv Environment

	// Check for local environment in the config file
	for _, env := range config.Environments {
		if env.EnvName == "local" {
			localEnv = env
		}
	}
	if localEnv.EnvName == "" {
		fmt.Println("config.json missing local environment")
		return
	}

	args := os.Args // e.g. go test -args beta
	if len(args) == 2 {
		for _, env := range config.Environments {
			if env.EnvName == args[1] {
				testEnv = env
			}
		}
	}

	// Default to local to handle "go test" with no args
	if testEnv.EnvName == "" {
		testEnv = localEnv
	}
}

// getClient creates an http client that does not follow redirects
func getClient() *http.Client {
	var netClient = &http.Client{
		Timeout: time.Second * 10,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return netClient
}

func setAuth(r *http.Request) {
	r.Header.Set(XAPIKEY, testEnv.APITestKey)
	r.Header.Set(XAPISECRET, testEnv.APITestSecret)
}

func TestBadQueueName(t *testing.T) {
	url := testEnv.APIURL + "/enq"

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	var task Task
	task.DelaySeconds = 1
	var headers []TaskHeader
	task.Headers = headers
	task.Payload = "ABC"
	task.QueueName = "InvalidName!"
	task.TimeoutSeconds = 5
	task.URL = testEnv.APIURL + "/test"

	jsonb, err := json.Marshal(task)
	if err != nil {
		t.Fatal("Unable to marshal test task")
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonb))
	setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotAcceptable {
		t.Fatalf("Did not get http.StatusNotAcceptable from %s: %s", url, body)
	}
}

func TestEnq(t *testing.T) {
	url := testEnv.APIURL + "/enq"

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	var task Task
	task.DelaySeconds = 1
	var headers []TaskHeader
	task.Headers = headers
	task.Payload = "ABC"
	task.QueueName = "default"
	task.TimeoutSeconds = 5
	task.URL = testEnv.APIURL + "/test"

	//fmt.Printf("%+v\n", task)

	jsonb, err := json.Marshal(task)
	if err != nil {
		t.Fatal("Unable to marshal test task")
	}

	//fmt.Printf("%s\n", string(jsonb))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonb))
	setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Did not get 200 OK from %s: %s", url, body)
	}
}

func TestEnqErr(t *testing.T) {
	url := testEnv.APIURL + "/enq"

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	var task Task
	task.DelaySeconds = 1
	var headers []TaskHeader
	task.Headers = headers
	task.Payload = "XYZ"
	task.QueueName = "crm"
	task.TimeoutSeconds = 5
	task.URL = testEnv.APIURL + "/testerr"

	//fmt.Printf("%+v\n", task)

	jsonb, err := json.Marshal(task)
	if err != nil {
		t.Fatal("Unable to marshal testerr task")
	}

	//fmt.Printf("%s\n", string(jsonb))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonb))
	setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Did not get 200 OK from %s: %s", url, body)
	}
}

func TestCounts(t *testing.T) {
	url := testEnv.APIURL + "/counts"

	req, err := http.NewRequest("GET", url, nil)
	setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	time.Sleep(500 * time.Millisecond) // Wait for counts to persist

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Did not get 200 OK from %s: %s", url, body)
	}

	var totals []CounterTotal
	err = json.Unmarshal(body, &totals)
	if err != nil {
		t.Fatal("Unable to unmarshal JSON CounterTotal")
	}

	if len(totals) == 0 {
		t.Fatal("Expected totals to have at least one entry")
	}

	for _, t := range totals {
		fmt.Println(t.Name, ": ", t.Total)
	}
}
