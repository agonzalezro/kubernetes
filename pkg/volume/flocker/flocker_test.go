package flocker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/volume"
)

func newInitializedVolumePlugMgr() volume.VolumePluginMgr {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/foo/bar", nil, nil))
	return plugMgr
}

func TestGetByName(t *testing.T) {
	assert := assert.New(t)
	plugMgr := newInitializedVolumePlugMgr()

	plug, err := plugMgr.FindPluginByName("kubernetes.io/flocker")
	assert.NotNil(plug, "Can't find the plugin by name")
	assert.Nil(err)
}

func TestCanSupportPersistentVolume(t *testing.T) {
	assert := assert.New(t)
	plugMgr := newInitializedVolumePlugMgr()

	plug, err := plugMgr.FindPluginByName("kubernetes.io/flocker")
	assert.Nil(err)

	assert.True(
		plug.CanSupport(
			&volume.Spec{
				PersistentVolume: &api.PersistentVolume{
					Spec: api.PersistentVolumeSpec{
						PersistentVolumeSource: api.PersistentVolumeSource{
							Flocker: &api.FlockerVolumeSource{},
						},
					},
				},
			},
		),
	)
}
