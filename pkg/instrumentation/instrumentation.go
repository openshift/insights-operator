package instrumentation

import (
	"encoding/json"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"time"
)

type OperatorConfiguration map[string]interface{}

func NewOperatorConfiguration() OperatorConfiguration {
	return make(map[string]interface{})
}

var configuration = NewOperatorConfiguration()

func init() {
}

func (configuration OperatorConfiguration) fromJSON(payload []byte) error {
	return json.Unmarshal(payload, &configuration)
}

func (configuration OperatorConfiguration) mergeWith(other OperatorConfiguration) {
	for key, value := range other {
		_, found := configuration[key]
		if found {
			configuration[key] = value
		}
	}
}

func (configuration OperatorConfiguration) print(title string) {
	klog.Info(title)
	for key, val := range configuration {
		klog.Info(key, "\t", val)
	}
}

func createOriginalConfiguration() OperatorConfiguration {
	var cfg = NewOperatorConfiguration()
	jsonStr := `
{"no_op":"?",
 "watch":[]}`
	cfg.fromJSON([]byte(jsonStr))
	return cfg
}

func retrieveConfigurationFrom(url string, cluster string) (OperatorConfiguration, error) {
	address := url + "/api/v1/operator/configuration/" + cluster

	request, err := http.NewRequest("GET", address, nil)
	if err != nil {
		klog.Error("Error: " + err.Error())
		return nil, err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		klog.Error("Error: " + err.Error())
		return nil, err
	}

	defer response.Body.Close()
	body, _ := ioutil.ReadAll(response.Body)

	var c2 = NewOperatorConfiguration()
	c2.fromJSON(body)
	return c2, nil
}

func StartInstrumentation(serviceUrl string, interval int) {
	c1 := createOriginalConfiguration()
	klog.Info("Gathering configuration each ", interval, " second(s)")
	for {
		klog.Info("Gathering info from service ", serviceUrl)
		c2, err := retrieveConfigurationFrom(serviceUrl, "cluster0")
		if err != nil {
			klog.Error("unable to retrieve configuration from the service")
		} else {
			c2.print("Retrieved configuration")

			c1.mergeWith(c2)
			c1.print("Updated configuration")
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
