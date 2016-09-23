package pushq

// goapp serve in a separate console window before running tests

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

// TODO - Automate creation of this key and secret... securely?

// Local Testing
const APIURL string = "http://localhost:8080"
const APITestKey string = "nXCfmjiMXvkDrCow"
const APITestSecret string = "qMUqFQjIHoCTuKZoHjsPNUXsknzloHfX"

// Beta Testing
/*
const APIURL string = "https://autoloop-pushq-beta.appspot.com"
const APITestKey string = "yxVSDcoezwUtWxDG"
const APITestSecret string = "rLmhkhFBZdSkkCSnUrCMpCvBBbVZOJEE"
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

func TestCounts(t *testing.T) {
	url := APIURL + "/counts"

	req, err := http.NewRequest("GET", url, nil)
	setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: time.Second * 10,
	}

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
