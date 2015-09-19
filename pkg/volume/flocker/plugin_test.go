package flocker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
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

func TestGetFlockerVolumeSource(t *testing.T) {
	assert := assert.New(t)

	p := flockerPlugin{}

	spec := &volume.Spec{
		Volume: &api.Volume{
			VolumeSource: api.VolumeSource{
				Flocker: &api.FlockerVolumeSource{},
			},
		},
	}
	vs, ro := p.getFlockerVolumeSource(spec)
	assert.False(ro)
	assert.Equal(vs, spec.Volume.Flocker)

	spec = &volume.Spec{
		PersistentVolume: &api.PersistentVolume{
			Spec: api.PersistentVolumeSpec{
				PersistentVolumeSource: api.PersistentVolumeSource{
					Flocker: &api.FlockerVolumeSource{},
				},
			},
		},
	}
	vs, ro = p.getFlockerVolumeSource(spec)
	assert.False(ro)
	assert.Equal(vs, spec.PersistentVolume.Spec.Flocker)
}

func TestNewBuilder(t *testing.T) {
	const expected = "expected-volume-name"
	assert := assert.New(t)

	plugMgr := newInitializedVolumePlugMgr()
	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.Nil(err)

	spec := &volume.Spec{
		Volume: &api.Volume{
			VolumeSource: api.VolumeSource{
				Flocker: &api.FlockerVolumeSource{Name: expected},
			},
		},
	}

	builder, err := plug.NewBuilder(spec, &api.Pod{}, volume.VolumeOptions{})
	assert.Nil(err)
	assert.Equal(builder.GetPath(), expected)
}

func TestNewCleaner(t *testing.T) {
	assert := assert.New(t)

	p := flockerPlugin{}

	cleaner, err := p.NewCleaner("", types.UID(""))
	assert.Nil(cleaner)
	assert.Nil(err)
}

func TestIsReadOnly(t *testing.T) {
	b := flockerBuilder{readOnly: true}
	assert.True(t, b.IsReadOnly())
}

func TestGetPath(t *testing.T) {
	const expected = "expected"
	b := flockerBuilder{flocker: &flocker{volName: expected}}
	assert.Equal(t, b.GetPath(), expected)
}
