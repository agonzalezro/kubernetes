package flocker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume"
)

func ProbeVolumePlugins() []volume.VolumePlugin {
	return []volume.VolumePlugin{&flockerPlugin{}}
}

type flockerPlugin struct {
	host volume.VolumeHost
}

func (p *flockerPlugin) Init(host volume.VolumeHost) {
	p.host = host
}

func (p flockerPlugin) Name() string {
	return "kubernetes.io/flocker"
}

func (p flockerPlugin) CanSupport(spec *volume.Spec) bool {
	// PersistenVolume is the only spec supported for now
	return spec.PersistentVolume != nil
}

func (p *flockerPlugin) getFlockerVolumeSource(spec *volume.Spec) (*api.FlockerVolumeSource, bool) {
	// AFAIK this will always be r/w, but perhaps for the future it will be needed
	readOnly := false

	if spec.Volume != nil && spec.Volume.Flocker != nil {
		return spec.Volume.Flocker, readOnly
	}
	return spec.PersistentVolume.Spec.Flocker, readOnly
}

func (p *flockerPlugin) NewBuilder(
	spec *volume.Spec, pod *api.Pod, opts volume.VolumeOptions, mounter mount.Interface,
) (volume.Builder, error) {
	source, readOnly := p.getFlockerVolumeSource(spec)
	builder := flockerBuilder{
		flocker: &flocker{
			volName: source.Name,
			pod:     pod,
			mounter: mounter,
			plugin:  p,
		},
		exe:      exec.New(),
		opts:     opts,
		readOnly: readOnly,
	}
	return &builder, nil
}

func (p *flockerPlugin) NewCleaner(
	volName string, podUID types.UID, mounter mount.Interface,
) (volume.Cleaner, error) {
	return nil, nil
}

// TODO: -- CUT HERE --

type flocker struct {
	volName   string
	datasetID string
	pod       *api.Pod
	mounter   mount.Interface
	plugin    *flockerPlugin
}

type flockerBuilder struct {
	*flocker
	exe      exec.Interface
	opts     volume.VolumeOptions
	readOnly bool
}

func (f flockerBuilder) GetPath() string {
	return f.volName
}

func (b flockerBuilder) SetUp() error {
	return b.SetUpAt(b.GetPath())
}

func (b flockerBuilder) SetUpAt(dir string) error {
	c, err := newFlockerClient(b.pod)
	if err != nil {
		return err
	}
	// The _ is the path, I don't think it's needed for anything here
	_, err = c.createVolume(dir)
	return err
}

func (b flockerBuilder) IsReadOnly() bool {
	return b.readOnly
}

// TODO: -- CUT HERE --

const (
	// From https://github.com/ClusterHQ/flocker-docker-plugin/blob/master/flockerdockerplugin/adapter.py#L18
	defaultVolumeSize = 107374182400

	keyFile  = "/etc/flocker/apiuser.key"
	certFile = "/etc/flocker/apiuser.crt"
	caFile   = "/etc/flocker/cluster.crt"

	timeoutWaitingForVolume = 30 * time.Second
)

type flockerClient struct {
	*http.Client

	pod *api.Pod

	host    string
	port    int
	version string

	maximumSize int

	ca, key, cert string // kubelet hardcoded paths
}

func newFlockerClient(pod *api.Pod) (*flockerClient, error) {
	host := os.Getenv("FLOCKER_CONTROL_SERVICE_HOST")
	if host == "" {
		return nil, errors.New("The environment variable FLOCKER_CONTROL_SERVICE_HOST can't be empty")
	}
	portEnv := os.Getenv("FLOCKER_CONTROL_SERVICE_PORT")
	port, err := strconv.Atoi(portEnv)
	if err != nil {
		return nil, fmt.Errorf("The environment variable FLOCKER_CONTROL_SERVICE_PORT needs to be a number, got: '%s'", portEnv)
	}

	return &flockerClient{
		Client:      &http.Client{},
		pod:         pod,
		host:        host,
		port:        port,
		version:     "v1",
		maximumSize: defaultVolumeSize,
		ca:          caFile,
		key:         keyFile,
		cert:        certFile,
	}, nil
}

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

func (c flockerClient) post(url string, payload interface{}) (*http.Response, error) {
	return c.request("POST", url, payload)
}

func (c flockerClient) get(url string) (*http.Response, error) {
	return c.request("GET", url, nil)
}

func (c flockerClient) getURL(path string) string {
	return fmt.Sprintf("https://%s:%d/%s/%s", c.host, c.port, c.version, path)
}

func (c flockerClient) checkVolumeErr(datasetID string) error {
	payload := struct {
		Primary string `json:"primary"`
	}{
		string(c.pod.UID),
	}

	resp, err := c.post(c.getURL(fmt.Sprintf("configuration/datasets/%s", datasetID)), payload)
	if err != nil {
		resp.Body.Close()
		return err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	if status < 200 || status > 299 {
		return fmt.Errorf("Expected: 2xx getting the volume, got: %d", status)
	}
	return nil
}

type configurationPayload struct {
	Primary     string          `json:"primary"`
	DatasetID   string          `json:"dataset_id"`
	MaximumSize int             `json:"maximum_size"`
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

// OMG, look at that name
func (c flockerClient) findDatasetIDByNameInConfigurationsPayload(body io.ReadCloser, name string) (dataset_id string, err error) {
	var result []configurationPayload
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		for _, r := range result {
			if r.Metadata.Name == name {
				return r.DatasetID, nil
			}
		}
	}
	return "", errors.New("Configuration not found by Name")
}

// OMG DITTO
func (c flockerClient) findStateByDatasetIDInStatesPayload(body io.ReadCloser, datasetID string) (path string, err error) {
	var result []statePayload
	if err = json.NewDecoder(body).Decode(&result); err != nil {
		for _, r := range result {
			if r.DatasetID == datasetID {
				return r.Path, nil
			}
		}
		return "", errors.New("State not found by Dataset ID")
	}
	return "", err
}

func (c flockerClient) getState(datasetID string) (*state, error) {
	resp, err := c.get(c.getURL("state/datasets"))
	if err != nil {
		resp.Body.Close()
		return nil, err
	}

	path, err := c.findStateByDatasetIDInStatesPayload(resp.Body, datasetID)
	if err != nil {
		return nil, err
	}

	return &state{
		DatasetID: datasetID,
		Path:      path,
	}, nil
}

/*
 * I feel a little bit guilty with this func, it should be refactored & tested later:
 *
 * 1) Get all the datasets
 * 2) If a dataset with that name/dir is found, return its path
 * 3) If not, create a new one
 * 4) Wait until the dataset is ready
 */
func (c flockerClient) createVolume(dir string) (path string, err error) {
	if strings.Contains(dir, "/") {
		return "", fmt.Errorf("The path (%s) can not contain the char '/'", dir)
	}

	payload := configurationPayload{
		Primary:     string(c.pod.UID),
		MaximumSize: defaultVolumeSize,
		Metadata: metadataPayload{
			Name: dir,
		},
	}

	// 1 & 2) Try to find the dataset if it was previously created
	resp, err := c.get(c.getURL("configuration/datasets"))
	if err != nil {
		resp.Body.Close()
		return "", err
	}
	defer resp.Body.Close()

	if datasetID, err := c.findDatasetIDByNameInConfigurationsPayload(resp.Body, dir); err == nil {
		state, err := c.getState(datasetID)
		if err != nil {
			return "", err
		}
		return state.Path, nil
	}

	// 3) Create a new one if we get here
	resp, err = c.post(c.getURL("configuration/datasets"), payload)
	if err != nil {
		resp.Body.Close()
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 { // TODO: Possible 409 race condition if we create the volume twice pretty quickly
		return "", fmt.Errorf("Expected: {1,2}xx creating the volume, got: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", nil
	}
	datasetID := payload.DatasetID

	// 4) Wait until the dataset is ready for usage, this can take a long a time
	if state, err := c.getState(datasetID); err != nil {
		timeoutChan := time.NewTimer(timeoutWaitingForVolume).C
		tickChan := time.NewTicker(500 * time.Millisecond).C

		for {
			select {
			case <-timeoutChan:
				// A goroutine can be running at this point, but it's not a big
				// deal. Worst case scenario we can use the context package
				return "", err
			case <-tickChan:
				if state, err := c.getState(datasetID); err == nil {
					return state.Path, nil
				}
			}
		}
	} else {
		return state.Path, nil
	}
}
