package flocker

import (
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
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
	return false
}

func (p flockerPlugin) NewBuilder(
	spec *volume.Spec, pod *api.Pod, _ volume.VolumeOptions, mounter mount.Interface,
) (volume.Builder, error) {
	return nil, nil
}

func (p *flockerPlugin) NewCleaner(
	volName string, podUID types.UID, mounter mount.Interface,
) (volume.Cleaner, error) {
	return nil, nil
}
