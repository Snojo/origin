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

package gce_pd

import (
	"os"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/mount"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/volume"
)

func TestCanSupport(t *testing.T) {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/tmp/fake", nil, nil))

	plug, err := plugMgr.FindPluginByName("kubernetes.io/gce-pd")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if plug.Name() != "kubernetes.io/gce-pd" {
		t.Errorf("Wrong name: %s", plug.Name())
	}
	if !plug.CanSupport(&volume.Spec{Name: "foo", VolumeSource: api.VolumeSource{GCEPersistentDisk: &api.GCEPersistentDiskVolumeSource{}}}) {
		t.Errorf("Expected true")
	}
}

func TestGetAccessModes(t *testing.T) {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/tmp/fake", nil, nil))

	plug, err := plugMgr.FindPersistentPluginByName("kubernetes.io/gce-pd")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if !contains(plug.GetAccessModes(), api.ReadWriteOnce) || !contains(plug.GetAccessModes(), api.ReadOnlyMany) {
		t.Errorf("Expected two AccessModeTypes:  %s and %s", api.ReadWriteOnce, api.ReadOnlyMany)
	}
}

func contains(modes []api.AccessModeType, mode api.AccessModeType) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}

type fakePDManager struct {
	attachCalled bool
	detachCalled bool
}

// TODO(jonesdl) To fully test this, we could create a loopback device
// and mount that instead.
func (fake *fakePDManager) AttachAndMountDisk(pd *gcePersistentDisk, globalPDPath string) error {
	globalPath := makeGlobalPDName(pd.plugin.host, pd.pdName)
	err := os.MkdirAll(globalPath, 0750)
	if err != nil {
		return err
	}
	fake.attachCalled = true
	// Simulate the global mount so that the fakeMounter returns the
	// expected number of mounts for the attached disk.
	pd.mounter.Mount(globalPath, globalPath, pd.fsType, nil)
	return nil
}

func (fake *fakePDManager) DetachDisk(pd *gcePersistentDisk) error {
	globalPath := makeGlobalPDName(pd.plugin.host, pd.pdName)
	err := os.RemoveAll(globalPath)
	if err != nil {
		return err
	}
	fake.detachCalled = true
	return nil
}

func TestPlugin(t *testing.T) {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/tmp/fake", nil, nil))

	plug, err := plugMgr.FindPluginByName("kubernetes.io/gce-pd")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	spec := &api.Volume{
		Name: "vol1",
		VolumeSource: api.VolumeSource{
			GCEPersistentDisk: &api.GCEPersistentDiskVolumeSource{
				PDName: "pd",
				FSType: "ext4",
			},
		},
	}
	fakeManager := &fakePDManager{}
	fakeMounter := &mount.FakeMounter{}
	builder, err := plug.(*gcePersistentDiskPlugin).newBuilderInternal(volume.NewSpecFromVolume(spec), types.UID("poduid"), fakeManager, fakeMounter)
	if err != nil {
		t.Errorf("Failed to make a new Builder: %v", err)
	}
	if builder == nil {
		t.Errorf("Got a nil Builder")
	}

	path := builder.GetPath()
	if path != "/tmp/fake/pods/poduid/volumes/kubernetes.io~gce-pd/vol1" {
		t.Errorf("Got unexpected path: %s", path)
	}

	if err := builder.SetUp(); err != nil {
		t.Errorf("Expected success, got: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Errorf("SetUp() failed, volume path not created: %s", path)
		} else {
			t.Errorf("SetUp() failed: %v", err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Errorf("SetUp() failed, volume path not created: %s", path)
		} else {
			t.Errorf("SetUp() failed: %v", err)
		}
	}
	if !fakeManager.attachCalled {
		t.Errorf("Attach watch not called")
	}

	fakeManager = &fakePDManager{}
	cleaner, err := plug.(*gcePersistentDiskPlugin).newCleanerInternal("vol1", types.UID("poduid"), fakeManager, fakeMounter)
	if err != nil {
		t.Errorf("Failed to make a new Cleaner: %v", err)
	}
	if cleaner == nil {
		t.Errorf("Got a nil Cleaner")
	}

	if err := cleaner.TearDown(); err != nil {
		t.Errorf("Expected success, got: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("TearDown() failed, volume path still exists: %s", path)
	} else if !os.IsNotExist(err) {
		t.Errorf("SetUp() failed: %v", err)
	}
	if !fakeManager.detachCalled {
		t.Errorf("Detach watch not called")
	}

}

func TestPluginLegacy(t *testing.T) {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/tmp/fake", nil, nil))

	plug, err := plugMgr.FindPluginByName("gce-pd")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if plug.Name() != "gce-pd" {
		t.Errorf("Wrong name: %s", plug.Name())
	}
	if plug.CanSupport(&volume.Spec{Name: "foo", VolumeSource: api.VolumeSource{GCEPersistentDisk: &api.GCEPersistentDiskVolumeSource{}}}) {
		t.Errorf("Expected false")
	}

	spec := &api.Volume{VolumeSource: api.VolumeSource{GCEPersistentDisk: &api.GCEPersistentDiskVolumeSource{}}}
	pod := &api.Pod{ObjectMeta: api.ObjectMeta{UID: types.UID("poduid")}}
	if _, err := plug.NewBuilder(volume.NewSpecFromVolume(spec), pod, volume.VolumeOptions{""}, nil); err == nil {
		t.Errorf("Expected failiure")
	}

	cleaner, err := plug.NewCleaner("vol1", types.UID("poduid"), nil)
	if err != nil {
		t.Errorf("Failed to make a new Cleaner: %v", err)
	}
	if cleaner == nil {
		t.Errorf("Got a nil Cleaner")
	}
}
