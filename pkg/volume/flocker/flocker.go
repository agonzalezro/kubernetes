package flocker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

func (p *flockerPlugin) NewBuilder(
	spec *volume.Spec, pod *api.Pod, opts volume.VolumeOptions, mounter mount.Interface,
) (volume.Builder, error) {
	builder := flockerBuilder{
		flocker: &flocker{
			volName: spec.Name(),
			pod:     pod,
			mounter: mounter,
			plugin:  p,
		},
		exe:  exec.New(),
		opts: opts,
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
	volName string
	pod     *api.Pod
	mounter mount.Interface
	plugin  *flockerPlugin
}

func (f flockerBuilder) GetPath() string {
	return f.volName
}

type flockerBuilder struct {
	*flocker
	exe  exec.Interface
	opts volume.VolumeOptions
}

func (b flockerBuilder) SetUp() error {
	return b.SetUpAt(b.GetPath())
}

func (b flockerBuilder) SetUpAt(dir string) error {
	c := newFlockerClient(b.pod)
	return c.createVolume(dir)
}

func (b flockerBuilder) IsReadOnly() bool {
	return false
}

// TODO: -- CUT HERE --

const (
	controlHost = "172.16.255.250"
	controlPort = 4523

	// TODO: From https://github.com/ClusterHQ/flocker-docker-plugin/blob/master/flockerdockerplugin/adapter.py#L18
	defaultVolumeSize = 107374182400

	keyFile  = "/etc/flocker/api.key"
	certFile = "/etc/flocker/api.crt"
	caFile   = "/etc/flocker/cluster.crt"
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

func newFlockerClient(pod *api.Pod) *flockerClient {
	return &flockerClient{
		Client:      &http.Client{},
		pod:         pod,
		host:        controlHost,
		port:        controlPort,
		version:     "v1",
		maximumSize: defaultVolumeSize,
		ca:          caFile,
		key:         keyFile,
		cert:        certFile,
	}
}

func (c flockerClient) post(url string, payload interface{}) (*http.Response, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// REMEMBER TO CLOSE THE BODY IN THE OUTSIDE FUNCTION
	return c.Do(req)
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

func (c flockerClient) createVolume(datasetID string) error {
	// TODO: probably, datasetID needs to be clean, ex: remove /
	payload := struct {
		Primary     string `json:"primary"`
		DatasetID   string `json:"dataset_id"`
		MaximumSize string `json:"maximum_size"`
	}{
		string(c.pod.UID),
		datasetID,
		c.MaximumSize,
	}

	// TODO: we could create 2 goroutines for create & try to get at the same time
	resp, err := c.post(c.getURL("configuration/datasets"), payload)
	if err != nil {
		resp.Body.Close()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("Expected: {1,2}xx creating the volume, got: %d", resp.StatusCode)
	}

	// Wait until the dataset is ready for usage, this can take a long a time
	if err := c.checkVolumeErr(datasetID); err != nil {
		// TODO: 30 is a magic number to wait for the volume
		timeoutChan := time.NewTimer(30 * time.Second).C
		tickChan := time.NewTicker(500 * time.Millisecond).C

		for {
			select {
			case <-timeoutChan:
				// TODO: a goroutine can be running at this point, but it's not a big deal
				return err
			case <-tickChan:
				if err := c.checkVolumeErr(datasetID); err == nil {
					return nil
				}
			}
		}
	}

	return nil
}
