package flocker

import (
	"bytes"
	"encoding/json"
	"net/http"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume"
)

// TODO: From https://github.com/ClusterHQ/flocker-docker-plugin/blob/master/flockerdockerplugin/adapter.py#L18
const defaultVolumeSize = 107374182400

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
	c := newFlockerClient()
	return c.createVolume(string(b.pod.UID), dir)
}

func (b flockerBuilder) IsReadOnly() bool {
	return false
}

// TODO: -- CUT HERE --

type flockerClient struct {
	*http.Client
}

func newFlockerClient() *flockerClient {
	return &flockerClient{&http.Client{}}
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

func (c flockerClient) createVolume(primary, datasetID string) error {
	payload := struct {
		Primary   string `json:"primary"`
		DatasetID string `json:"dataset_id"`
	}{
		primary,
		datasetID,
	}

	resp, err := c.post(URL_HERE, payload)
	if err != nil {
		resp.Body.Close()
		return err
	}
	defer resp.Body.Close()
	// READ JSON RESPONSE FROM FLOCKER

	return nil
}
