package instrumentation

import (
	"encoding/json"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"sort"
	"time"
)

// An unstructured operator configuration that can contain
// any data stored under (string) keys.
type OperatorConfiguration map[string]interface{}

// Constructor for the operator configuration.
func NewOperatorConfiguration() OperatorConfiguration {
	return make(map[string]interface{})
}

var configuration = NewOperatorConfiguration()

func init() {
}

func (configuration OperatorConfiguration) fromJSON(payload []byte) error {
	return json.Unmarshal(payload, &configuration)
}

func (configuration OperatorConfiguration) addAll(other OperatorConfiguration) {
	for key, value := range other {
		configuration[key] = value
	}
}

func (configuration OperatorConfiguration) updateExisting(other OperatorConfiguration) {
	for key, value := range other {
		_, found := configuration[key]
		if found {
			configuration[key] = value
		}
	}
}

func (configuration OperatorConfiguration) mergeWith(other OperatorConfiguration) {
	if len(configuration) == 0 {
		configuration.addAll(other)
	} else {
		configuration.updateExisting(other)
	}
}

// Print the configuration. Items are sorted by its keys.
func (configuration OperatorConfiguration) print(title string) {
	klog.Info(title)
	if len(configuration) == 0 {
		klog.Info("\t* empty *")
		return
	}

	var keys []string
	for key, _ := range configuration {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		klog.Info("\t", key, "\t=> ", configuration[key])
	}
}

// Create original operator configuration.
func createOriginalConfiguration(filename string) OperatorConfiguration {
	var cfg = NewOperatorConfiguration()

	payload, err := ioutil.ReadFile(filename)
	if err != nil {
		klog.Error("Can not open configuration file: ", err)
		// ok for now, the configuration will be simply empty
		return cfg
	}

	err = cfg.fromJSON(payload)
	if err != nil {
		klog.Warning("Can not decode original configuration read from the file ", filename)
		// ok for now, the configuration will be simply empty
		return cfg
	}
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

	if response.StatusCode != http.StatusOK {
		klog.Info("No configuration has been provided by the service")
		return nil, nil
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var c2 = NewOperatorConfiguration()
	err = c2.fromJSON(body)
	if err != nil {
		klog.Warning("Can not decode the configuration provided by the service")
		return nil, err
	}
	return c2, nil
}

func StartInstrumentation(serviceUrl string, interval int, clusterName string, configFile string) {
	klog.Info("Read original configuration")
	c1 := createOriginalConfiguration(configFile)
	c1.print("Original configuration")
	klog.Info("Gathering configuration each ", interval, " second(s)")
	for {
		klog.Info("Gathering info from service ", serviceUrl)
		c2, err := retrieveConfigurationFrom(serviceUrl, clusterName)
		if err != nil {
			klog.Error("unable to retrieve configuration from the service")
		} else if c2 != nil {
			c2.print("Retrieved configuration")
			c1.mergeWith(c2)
			c1.print("Updated configuration")
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
