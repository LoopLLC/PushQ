package pushq

// goapp serve in a separate console window before running tests

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

// TODO - Automate creation of this key and secret... securely?

// Local Testing
const APIURL string = "http://localhost:8080"
const APITestKey string = "HNoGuLrPXPEWTPkC"
const APITestSecret string = "sHXzRppJhsaqXuQzPbbbUfCtytyICNFD"

// Beta Testing
/*
const APIURL string = "https://autoloop-pushq-beta.appspot.com"
const APITestKey string = "VagxTGqWXjdzIzHn"
const APITestSecret string = "JfgZecGqoiOuIawQQTQzGbPvmoPVAgDq"
*/

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
	r.Header.Set(XAPIKEY, APITestKey)
	r.Header.Set(XAPISECRET, APITestSecret)
}

func TestBadQueueName(t *testing.T) {
	url := APIURL + "/enq"

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
	task.URL = APIURL + "/test"

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
	url := APIURL + "/enq"

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
	task.URL = APIURL + "/test"

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

func TestCounts(t *testing.T) {
	url := APIURL + "/counts"

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

	//for _, t := range totals {
	//	fmt.Println(t.Name, ": ", t.Total)
	//}
}
