package flocker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/volume"
)

const pluginName = "kubernetes.io/flocker"

func newInitializedVolumePlugMgr() volume.VolumePluginMgr {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/foo/bar", nil, nil))
	return plugMgr
}

func TestGetByName(t *testing.T) {
	assert := assert.New(t)
	plugMgr := newInitializedVolumePlugMgr()

	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.NotNil(plug, "Can't find the plugin by name")
	assert.Nil(err)
}

func TestCanSupport(t *testing.T) {
	assert := assert.New(t)
	plugMgr := newInitializedVolumePlugMgr()

	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.Nil(err)

	specs := map[*volume.Spec]bool{
		&volume.Spec{
			Volume: &api.Volume{
				VolumeSource: api.VolumeSource{
					Flocker: &api.FlockerVolumeSource{},
				},
			},
		}: true,
		&volume.Spec{
			PersistentVolume: &api.PersistentVolume{
				Spec: api.PersistentVolumeSpec{
					PersistentVolumeSource: api.PersistentVolumeSource{
						Flocker: &api.FlockerVolumeSource{},
					},
				},
			},
		}: true,
		&volume.Spec{
			Volume: &api.Volume{
				VolumeSource: api.VolumeSource{},
			},
		}: false,
	}

	for spec, expected := range specs {
		actual := plug.CanSupport(spec)
		assert.Equal(expected, actual)
	}
}
