package flocker

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"k8s.io/kubernetes/pkg/api"
)

const (
	// From https://github.com/ClusterHQ/flocker-docker-plugin/blob/master/flockerdockerplugin/adapter.py#L18
	defaultVolumeSize = float32(107374182400)

	// Flocker connections are authenticated with TLS
	// TODO: It can perhaps be stored somewhere else.
	defaultCACertFile     = "/etc/flocker/cluster.crt"
	defaultClientKeyFile  = "/etc/flocker/apiuser.key"
	defaultClientCertFile = "/etc/flocker/apiuser.crt"
)

var (
	// A volume can take a long time to be available, if we don't want
	// Kubernetes to wait forever we need to stop trying after some time, that
	// time is defined here
	timeoutWaitingForVolume = 2 * time.Minute
	tickerWaitingForVolume  = 5 * time.Second
)

func newTLSClient(caCertPath, keyPath, certPath string) (*http.Client, error) {
	// Client certificate
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	// CA certificate
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{TLSClientConfig: tlsConfig}

	return &http.Client{Transport: transport}, nil
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// TODO: use this
type flockerClientable interface {
	httpDoer
	CreateVolume(string) (string, error)
}

type flockerClient struct {
	*http.Client

	pod *api.Pod

	schema  string
	host    string
	port    int
	version string

	maximumSize float32
}

var (
	errFlockerControlServiceHost = errors.New("The environment variable FLOCKER_CONTROL_SERVICE_HOST can't be empty")
	errFlockerControlServicePort = errors.New("The environment variable FLOCKER_CONTROL_SERVICE_PORT must be a number")
)

/*
 * newFlockerClient creates a wrapper over http.Client to communicate with the
 * flocker control service. The location of this service is defined by the
 * following environment variables:
 *
 * - FLOCKER_CONTROL_SERVICE_HOST
 * - FLOCKER_CONTROL_SERVICE_PORT
 *
 * If you are using the TLS client you can also specify where the cert files
 * reside:
 *
 * - FLOCKER_CONTROL_SERVICE_CA_FILE
 * - FLOCKER_CONTROL_SERVICE_CLIENT_KEY_FILE
 * - FLOCKER_CONTROL_SERVICE_CLIENT_CERT_FILE
 */
func newFlockerClient(pod *api.Pod, useTLS bool) (*flockerClient, error) {
	host := os.Getenv("FLOCKER_CONTROL_SERVICE_HOST")
	if host == "" {
		return nil, errFlockerControlServiceHost
	}
	portEnv := os.Getenv("FLOCKER_CONTROL_SERVICE_PORT")
	port, err := strconv.Atoi(portEnv)
	if err != nil {
		return nil, errFlockerControlServicePort
	}

	var client *http.Client
	if useTLS {
		client, err = newTLSClient(
			GetenvOrFallback("FLOCKER_CONTROL_SERVICE_CA_FILE", defaultCACertFile),
			GetenvOrFallback("FLOCKER_CONTROL_SERVICE_CLIENT_KEY_FILE", defaultClientKeyFile),
			GetenvOrFallback("FLOCKER_CONTROL_SERVICE_CLIENT_CERT_FILE", defaultClientCertFile),
		)
		if err != nil {
			return nil, err
		}
	} else {
		client = &http.Client{}
	}

	return &flockerClient{
		Client:      client,
		pod:         pod,
		schema:      "https",
		host:        host,
		port:        port,
		version:     "v1",
		maximumSize: defaultVolumeSize,
	}, nil
}

/*
 * request do a request using the http.Client embedded to the control service
 * and returns the response or an error in case it happens.
 *
 * Note: you will need to deal with the response body call to Close if you
 * don't want to deal with problems later.
 */
func (c flockerClient) request(method, url string, payload interface{}) (*http.Response, error) {
	var (
		b   []byte
		err error
	)

	if method == "POST" { // Just allow payload on POST
		b, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// REMEMBER TO CLOSE THE BODY IN THE OUTSIDE FUNCTION
	return c.Do(req)
}

// post performs a post request with the indicated payload
func (c flockerClient) post(url string, payload interface{}) (*http.Response, error) {
	return c.request("POST", url, payload)
}

// get performs a get request
func (c flockerClient) get(url string) (*http.Response, error) {
	return c.request("GET", url, nil)
}

// getURL returns a full URI to the control service
func (c flockerClient) getURL(path string) string {
	return fmt.Sprintf("%s://%s:%d/%s/%s", c.schema, c.host, c.port, c.version, path)
}

type configurationPayload struct {
	Primary     string          `json:"primary"`
	DatasetID   string          `json:"dataset_id"`
	MaximumSize float32         `json:"maximum_size"`
	Metadata    metadataPayload `json:"metadata"`
}

type metadataPayload struct {
	Name string `json:"name"`
}

type state struct {
	Path      string `json:"path"`
	DatasetID string `json:"dataset_id"`
}

type statePayload struct {
	*state
}

// findIDInConfigurationsPayload returns the datasetID if it was found in the
// configurations payload, otherwise it will return an error.
func (c flockerClient) findIDInConfigurationsPayload(body io.ReadCloser, name string) (datasetID string, err error) {
	var configurations []configurationPayload
	if err = json.NewDecoder(body).Decode(&configurations); err == nil {
		for _, r := range configurations {
			if r.Metadata.Name == name {
				return r.DatasetID, nil
			}
		}
		return "", errors.New("Configuration not found by Name")
	}
	return "", err
}

// findPathInStatesPayload returns the path of the given datasetID if it was
// found in the states payload. In case the path is not found it returns an
// error.
func (c flockerClient) findPathInStatesPayload(body io.ReadCloser, datasetID string) (path string, err error) {
	var states []statePayload
	if err = json.NewDecoder(body).Decode(&states); err == nil {
		for _, r := range states {
			if r.DatasetID == datasetID {
				return r.Path, nil
			}
		}
		return "", errors.New("State not found by Dataset ID")
	}
	return "", err
}

// getState performs a get request to get the state of the given datasetID, if
// something goes wrong or the datasetID was not found it returns an error.
func (c flockerClient) getState(datasetID string) (*state, error) {
	resp, err := c.get(c.getURL("state/datasets"))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	path, err := c.findPathInStatesPayload(resp.Body, datasetID)
	if err != nil {
		return nil, err
	}

	return &state{
		DatasetID: datasetID,
		Path:      path,
	}, nil
}

/*
 * CreateVolume creates a volume in Flocker and waits for it to be ready, this
 * process is a little bit complex but follows this flow:
 *
 * 1) Get all the datasets
 * 2) If a dataset with that name/dir is found, return its path
 * 3) If not, create a new one
 * 4) Wait until the dataset is ready or the timeout was reached
 */
func (c flockerClient) CreateVolume(dir string) error {
	// 1 & 2) Try to find the dataset if it was previously created
	resp, err := c.get(c.getURL("configuration/datasets"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if datasetID, err := c.findIDInConfigurationsPayload(resp.Body, dir); err == nil {
		state, _ := c.getState(datasetID)
		// If it fails here it can be recovered, this is why the error is ignored
		if state.Path == dir {
			return nil
		}
	}

	payload := configurationPayload{
		Primary:     "1bc464e3-3354-4d2f-adf4-82f93ae0016f", // TODO: use /v1/state/nodes
		MaximumSize: defaultVolumeSize,
		Metadata: metadataPayload{
			Name: dir,
		},
	}

	// 3) Create a new one if we get here
	resp, err = c.post(c.getURL("configuration/datasets"), payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 { // TODO: Possible 409 race condition if we create the volume twice pretty quickly
		return fmt.Errorf("Expected: {1,2}xx creating the volume, got: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	datasetID := payload.DatasetID

	// 4) Wait until the dataset is ready for usage, this can take a long a time
	if _, err := c.getState(datasetID); err != nil {
		timeoutChan := time.NewTimer(timeoutWaitingForVolume).C
		tickChan := time.NewTicker(tickerWaitingForVolume).C

		for {
			select {
			case <-timeoutChan:
				// A goroutine can be running at this point, but it's not a big
				// deal. Worst case scenario we can use the context package
				return err
			case <-tickChan:
				if _, err := c.getState(datasetID); err == nil {
					return nil
				}
			}
		}
	} else {
		return nil
	}
}
