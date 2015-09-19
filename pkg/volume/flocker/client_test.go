package flocker

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindIDInConfigurationsPayload(t *testing.T) {
	const (
		searched_name = "search-for-this-name"
		expected      = "The-42-id"
	)
	assert := assert.New(t)

	c := flockerClient{}

	payload := fmt.Sprintf(
		`[{"dataset_id": "1-2-3", "metadata": {"name": "test"}}, {"dataset_id": "The-42-id", "metadata": {"name": "%s"}}]`,
		searched_name,
	)

	id, err := c.findIDInConfigurationsPayload(
		ioutil.NopCloser(bytes.NewBufferString(payload)), searched_name,
	)
	assert.Nil(err)
	assert.Equal(id, expected)
}

func TestFindPathInStatesPayload(t *testing.T) {
	const (
		searched_id = "search-for-this-dataset-id"
		expected    = "awesome-path"
	)
	assert := assert.New(t)

	c := flockerClient{}

	payload := fmt.Sprintf(
		`[{"dataset_id": "1-2-3", "path": "not-this-one"}, {"dataset_id": "%s", "path": "awesome-path"}]`,
		searched_id,
	)
	path, err := c.findPathInStatesPayload(
		ioutil.NopCloser(bytes.NewBufferString(payload)), searched_id,
	)
	assert.Nil(err)
	assert.Equal(path, expected)
}
