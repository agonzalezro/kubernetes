package flocker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

		_, err := newFlockerClient(pod)
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

	c, err := newFlockerClient(&api.Pod{})
	assert.Nil(err)

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

	c, err := newFlockerClient(&api.Pod{})
	assert.Nil(err)

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
}

func TestGetURL(t *testing.T) {
	const expected = "https://host:42/v1/test"
	assert := assert.New(t)

	os.Setenv("FLOCKER_CONTROL_SERVICE_HOST", "host")
	os.Setenv("FLOCKER_CONTROL_SERVICE_PORT", "42")
	c, err := newFlockerClient(&api.Pod{})
	assert.Nil(err)

	url := c.getURL("test")
	assert.Equal(url, expected)
}
