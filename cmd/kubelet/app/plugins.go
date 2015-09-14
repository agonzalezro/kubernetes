/*
Copyright 2014 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

// This file exists to force the desired plugin implementations to be linked.
import (
	// Credential providers
	_ "k8s.io/kubernetes/pkg/credentialprovider/gcp"
	// Network plugins
	"k8s.io/kubernetes/pkg/kubelet/network"
	"k8s.io/kubernetes/pkg/kubelet/network/exec"
	// Volume plugins
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/aws_ebs"
	"k8s.io/kubernetes/pkg/volume/cephfs"
	"k8s.io/kubernetes/pkg/volume/cinder"
	"k8s.io/kubernetes/pkg/volume/downwardapi"
	"k8s.io/kubernetes/pkg/volume/empty_dir"
	"k8s.io/kubernetes/pkg/volume/flocker"
	"k8s.io/kubernetes/pkg/volume/gce_pd"
	"k8s.io/kubernetes/pkg/volume/git_repo"
	"k8s.io/kubernetes/pkg/volume/glusterfs"
	"k8s.io/kubernetes/pkg/volume/host_path"
	"k8s.io/kubernetes/pkg/volume/iscsi"
	"k8s.io/kubernetes/pkg/volume/nfs"
	"k8s.io/kubernetes/pkg/volume/persistent_claim"
	"k8s.io/kubernetes/pkg/volume/rbd"
	"k8s.io/kubernetes/pkg/volume/secret"
	//Cloud providers
	_ "k8s.io/kubernetes/pkg/cloudprovider/providers"
)

// ProbeVolumePlugins collects all volume plugins into an easy to use list.
func ProbeVolumePlugins() []volume.VolumePlugin {
	allPlugins := []volume.VolumePlugin{}

	// The list of plugins to probe is decided by the kubelet binary, not
	// by dynamic linking or other "magic".  Plugins will be analyzed and
	// initialized later.
	//
	// Kubelet does not currently need to configure volume plugins.
	// If/when it does, see kube-controller-manager/app/plugins.go for example of using volume.VolumeConfig
	for _, plugins := range [][]volume.VolumePlugin{
		aws_ebs.ProbeVolumePlugins(),
		empty_dir.ProbeVolumePlugins(),
		gce_pd.ProbeVolumePlugins(),
		git_repo.ProbeVolumePlugins(),
		host_path.ProbeVolumePlugins(volume.VolumeConfig{}),
		nfs.ProbeVolumePlugins(volume.VolumeConfig{}),
		secret.ProbeVolumePlugins(),
		iscsi.ProbeVolumePlugins(),
		glusterfs.ProbeVolumePlugins(),
		persistent_claim.ProbeVolumePlugins(),
		rbd.ProbeVolumePlugins(),
		cinder.ProbeVolumePlugins(),
		cephfs.ProbeVolumePlugins(),
		downwardapi.ProbeVolumePlugins(),
		flocker.ProbeVolumePlugins(),
	} {
		allPlugins = append(allPlugins, plugins...)
	}
	return allPlugins
}

// ProbeNetworkPlugins collects all compiled-in plugins
func ProbeNetworkPlugins(pluginDir string) []network.NetworkPlugin {
	allPlugins := []network.NetworkPlugin{}

	// for each existing plugin, add to the list
	allPlugins = append(allPlugins, exec.ProbeNetworkPlugins(pluginDir)...)

	return allPlugins
}
