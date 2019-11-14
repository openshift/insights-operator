package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"net/http"
	"io"
	"io/ioutil"
	"crypto/tls"
)

const API_PREFIX = "/api/v1/"

type Trigger struct {
	Id          int    `json:"id"`
	Type        string `json:"type"`
	Cluster     string `json:"cluster"`
	Reason      string `json:"reason"`
	Link        string `json:"link"`
	TriggeredAt string `json:"triggered_at"`
	TriggeredBy string `json:"triggered_by"`
	Parameters  string `json:"parameters"`
	Active      int    `json:"active"`
}

func performReadRequest(url string) ([]byte, error) {
	if os.Getenv("ENV") == "dev" {
	    http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	response, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Communication error with the server %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Expected HTTP status 200 OK, got %d", response.StatusCode)
	}
	body, readErr := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if readErr != nil {
		return nil, fmt.Errorf("Unable to read response body")
	}

	return body, nil
}

func performWriteRequest(url string, method string, payload io.Reader) error {
	var client http.Client
	if os.Getenv("ENV") == "dev" {
	    http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return fmt.Errorf("Error creating request %v", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Communication error with the server %v", err)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Expected HTTP status 200 OK, 201 Created or 202 Accepted, got %d", response.StatusCode)
	}
	return nil
}

func readListOfTriggers(controllerUrl string, apiPrefix string, clusterName string) ([]Trigger, error) {
	var triggers []Trigger
	url := controllerUrl + apiPrefix + "operator/triggers/" + clusterName
	body, err := performReadRequest(url)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &triggers)
	if err != nil {
		return nil, err
	}
	return triggers, nil
}

func ackTrigger(controllerUrl string, apiPrefix string, clusterName string, triggerId string) error {
	url := controllerUrl + apiPrefix + "operator/trigger/" + clusterName + "/ack/" + triggerId
	err := performWriteRequest(url, "PUT", nil)
	return err
}
