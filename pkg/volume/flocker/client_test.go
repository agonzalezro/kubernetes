package flocker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/kubernetes/pkg/api"

	"github.com/stretchr/testify/assert"
)

func TestNewFlockerClient(t *testing.T) {
	assert := assert.New(t)

	pod := &api.Pod{}

	var tests = map[error][]string{
		errFlockerControlServiceHost: []string{"", "1"},
		errFlockerControlServicePort: []string{"host", "fail"},
		nil: []string{"host", "1"},
	}

	for expectedErr, envs := range tests {
		os.Setenv("FLOCKER_CONTROL_SERVICE_HOST", envs[0])
		os.Setenv("FLOCKER_CONTROL_SERVICE_PORT", envs[1])

		_, err := newFlockerClient(pod, false)
		assert.Equal(err, expectedErr)
	}
}

func TestPost(t *testing.T) {
	const (
		expectedPayload    = "foobar"
		expectedStatusCode = 418
	)

	assert := assert.New(t)

	type payload struct {
		Test string `json:"test"`
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result payload
		err := json.NewDecoder(r.Body).Decode(&result)
		assert.Nil(err)
		assert.Equal(result.Test, expectedPayload)
		w.WriteHeader(expectedStatusCode)
	}))
	defer ts.Close()

	c := flockerClient{Client: &http.Client{}}

	resp, err := c.post(ts.URL, payload{expectedPayload})
	assert.Nil(err)
	assert.Equal(resp.StatusCode, expectedStatusCode)
}

func TestGet(t *testing.T) {
	const (
		expectedStatusCode = 418
	)

	assert := assert.New(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(expectedStatusCode)
	}))
	defer ts.Close()

	c := flockerClient{Client: &http.Client{}}

	resp, err := c.get(ts.URL)
	assert.Nil(err)
	assert.Equal(resp.StatusCode, expectedStatusCode)
}

func TestFindIDInConfigurationsPayload(t *testing.T) {
	const (
		searchedName = "search-for-this-name"
		expected     = "The-42-id"
	)
	assert := assert.New(t)

	c := flockerClient{}

	payload := fmt.Sprintf(
		`[{"dataset_id": "1-2-3", "metadata": {"name": "test"}}, {"dataset_id": "The-42-id", "metadata": {"name": "%s"}}]`,
		searchedName,
	)

	id, err := c.findIDInConfigurationsPayload(
		ioutil.NopCloser(bytes.NewBufferString(payload)), searchedName,
	)
	assert.Nil(err)
	assert.Equal(id, expected)

	id, err = c.findIDInConfigurationsPayload(
		ioutil.NopCloser(bytes.NewBufferString(payload)), "it will not be found",
	)
	assert.NotNil(err)

	id, err = c.findIDInConfigurationsPayload(
		ioutil.NopCloser(bytes.NewBufferString("invalid { json")), "",
	)
	assert.NotNil(err)
}

func TestFindPathInStatesPayload(t *testing.T) {
	const (
		searchedID = "search-for-this-dataset-id"
		expected   = "awesome-path"
	)
	assert := assert.New(t)

	c := flockerClient{}

	payload := fmt.Sprintf(
		`[{"dataset_id": "1-2-3", "path": "not-this-one"}, {"dataset_id": "%s", "path": "awesome-path"}]`,
		searchedID,
	)
	path, err := c.findPathInStatesPayload(
		ioutil.NopCloser(bytes.NewBufferString(payload)), searchedID,
	)
	assert.Nil(err)
	assert.Equal(path, expected)

	path, err = c.findPathInStatesPayload(
		ioutil.NopCloser(bytes.NewBufferString(payload)), "this is not going to be there",
	)
	assert.NotNil(err)

	path, err = c.findPathInStatesPayload(
		ioutil.NopCloser(bytes.NewBufferString("not even } json")), "",
	)
	assert.NotNil(err)
}

func TestGetURL(t *testing.T) {
	const expected = "https://host:42/v1/test"
	assert := assert.New(t)

	os.Setenv("FLOCKER_CONTROL_SERVICE_HOST", "host")
	os.Setenv("FLOCKER_CONTROL_SERVICE_PORT", "42")
	c, err := newFlockerClient(&api.Pod{}, false)
	assert.Nil(err)

	url := c.getURL("test")
	assert.Equal(url, expected)
}

func getHostAndPortFromTestServer(ts *httptest.Server) (string, int, error) {
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		return "", 0, err
	}

	hostSplits := strings.Split(tsURL.Host, ":")

	port, err := strconv.Atoi(hostSplits[1])
	if err != nil {
		return "", 0, nil
	}
	return hostSplits[0], port, nil
}

func setClientEnvVars(host string, port int) {
	os.Setenv("FLOCKER_CONTROL_SERVICE_HOST", host)
	os.Setenv("FLOCKER_CONTROL_SERVICE_PORT", strconv.Itoa(port))
}

func TestHappyPathCreateVolumeFromNonExistent(t *testing.T) {
	const expected = "dir"

	assert := assert.New(t)
	var (
		numCalls int
		err      error
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		numCalls++
		switch numCalls {
		case 1:
			assert.Equal(r.Method, "GET")
			assert.Equal(r.URL.Path, "/v1/configuration/datasets")
		case 2:
			assert.Equal(r.Method, "POST")
			assert.Equal(r.URL.Path, "/v1/configuration/datasets")
			w.Write([]byte(`{"dataset_id": "123"}`))
			// TODO: test payload
		case 3:
			assert.Equal(r.Method, "GET")
			assert.Equal(r.URL.Path, "/v1/state/datasets")
		case 4:
			assert.Equal(r.Method, "GET")
			assert.Equal(r.URL.Path, "/v1/state/datasets")
			w.Write([]byte(fmt.Sprintf(`[{"dataset_id": "123", "path": "%s"}]`, expected)))
		}
	}))

	host, port, err := getHostAndPortFromTestServer(ts)
	assert.Nil(err)
	setClientEnvVars(host, port)

	c, err := newFlockerClient(&api.Pod{}, false)
	assert.Nil(err)
	c.schema = "http"
	tickerWaitingForVolume = 1 * time.Millisecond // TODO: this is overriding globally

	err = c.CreateVolume(expected)
	assert.Nil(err)
}

func TestCreateVolumeThatAlreadyExists(t *testing.T) {
	const expected = "dir"

	assert := assert.New(t)
	var numCalls int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		numCalls++
		switch numCalls {
		case 1:
			w.Write([]byte(fmt.Sprintf(`[{"dataset_id": "123", "metadata": {"name": "%s"}}]`, expected)))
		case 2:
			w.Write([]byte(fmt.Sprintf(`[{"dataset_id": "123", "path": "%s"}]`, expected)))
		}
	}))

	host, port, err := getHostAndPortFromTestServer(ts)
	assert.Nil(err)
	setClientEnvVars(host, port)

	c, err := newFlockerClient(&api.Pod{}, false)
	assert.Nil(err)
	c.schema = "http"

	err = c.CreateVolume(expected)
	assert.Nil(err)
}
