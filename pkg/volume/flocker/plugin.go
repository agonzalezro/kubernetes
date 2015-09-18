package flocker

import (
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

func (p *flockerPlugin) Init(host volume.VolumeHost) {
	p.host = host
}

func (p flockerPlugin) Name() string {
	return "kubernetes.io/flocker"
}

func (p flockerPlugin) CanSupport(spec *volume.Spec) bool {
	return (spec.PersistentVolume != nil && spec.PersistentVolume.Spec.Flocker != nil) ||
		(spec.Volume != nil && spec.Volume.Flocker != nil)
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
