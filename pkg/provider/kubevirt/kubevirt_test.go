package kubevirt

import (
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeProvider() *Provider {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstance"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstanceList"},
		&unstructured.UnstructuredList{},
	)
	client := dynamicfake.NewSimpleDynamicClient(scheme)
	return NewFromClient(client, "test-ns")
}

func TestProviderName(t *testing.T) {
	p := newFakeProvider()
	if p.Name() != "kubevirt" {
		t.Errorf("expected name 'kubevirt', got %q", p.Name())
	}
}

func TestCapabilities(t *testing.T) {
	p := newFakeProvider()
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != types.ResourceTypeVM {
		t.Errorf("expected [vm], got %v", caps)
	}
}

func TestPlanCreate(t *testing.T) {
	p := newFakeProvider()
	desired := &types.Resource{
		Name: "myapp-webserver",
		Type: types.ResourceTypeVM,
		Properties: map[string]any{
			"image": "quay.io/containerdisks/fedora:latest",
			"cpu":   2,
		},
	}

	diff, err := p.Plan(desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionCreate {
		t.Errorf("expected create, got %s", diff.Action)
	}
}

func TestPlanUpdate(t *testing.T) {
	p := newFakeProvider()
	current := &types.Resource{
		Name: "myapp-webserver-abc12345",
		Type: types.ResourceTypeVM,
		Properties: map[string]any{
			"image": "quay.io/containerdisks/fedora:latest",
			"cpu":   1,
		},
	}
	desired := &types.Resource{
		Name: "myapp-webserver",
		Type: types.ResourceTypeVM,
		Properties: map[string]any{
			"image": "quay.io/containerdisks/fedora:latest",
			"cpu":   4,
		},
	}

	diff, err := p.Plan(desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionUpdate {
		t.Errorf("expected update, got %s", diff.Action)
	}
	if diff.Resource != "myapp-webserver-abc12345" {
		t.Errorf("expected update to use current name, got %q", diff.Resource)
	}
}

func TestPlanNoChange(t *testing.T) {
	p := newFakeProvider()
	resource := &types.Resource{
		Name: "myapp-webserver-abc12345",
		Type: types.ResourceTypeVM,
		Properties: map[string]any{
			"image": "quay.io/containerdisks/fedora:latest",
		},
	}

	diff, err := p.Plan(resource, resource)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionNone {
		t.Errorf("expected none, got %s", diff.Action)
	}
}

func TestApplyCreate(t *testing.T) {
	p := newFakeProvider()
	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "myapp-webserver",
		Type:     types.ResourceTypeVM,
		Provider: "kubevirt",
		After: map[string]any{
			"image":    "quay.io/containerdisks/fedora:latest",
			"cpu":      2,
			"memoryMB": 2048,
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(resource.Name, "myapp-webserver-") {
		t.Errorf("expected name to start with 'myapp-webserver-', got %q", resource.Name)
	}
	if resource.Status != types.ResourceStatusReady {
		t.Errorf("expected ready status, got %s", resource.Status)
	}
	if resource.Outputs["namespace"] != "test-ns" {
		t.Errorf("expected namespace test-ns, got %v", resource.Outputs["namespace"])
	}
}

func TestApplyCreateAndDestroy(t *testing.T) {
	p := newFakeProvider()
	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "myapp-vm",
		Type:     types.ResourceTypeVM,
		Provider: "kubevirt",
		After: map[string]any{
			"image": "quay.io/containerdisks/fedora:latest",
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	// Destroy it.
	if err := p.Destroy(resource); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	// Status should be unknown after destroy.
	status, err := p.Status(resource)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.ResourceStatusUnknown {
		t.Errorf("expected unknown after destroy, got %s", status)
	}
}

func TestApplyRequiresImage(t *testing.T) {
	p := newFakeProvider()
	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "myapp-vm",
		Type:     types.ResourceTypeVM,
		Provider: "kubevirt",
		After:    map[string]any{"cpu": 2},
	}

	_, err := p.Apply(diff)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestBuildVMStructure(t *testing.T) {
	labels := map[string]any{
		"app.kubernetes.io/name": "test-vm",
	}
	vm := buildVM("test-vm", "default", labels, "quay.io/containerdisks/fedora:latest", 4, "4Gi", true, map[string]any{})

	// Verify basic structure.
	spec, ok, _ := unstructured.NestedMap(vm.Object, "spec")
	if !ok {
		t.Fatal("missing spec")
	}

	running, ok, _ := unstructured.NestedBool(spec, "running")
	if !ok || !running {
		t.Error("expected running=true")
	}

	cores, ok, _ := unstructured.NestedFieldNoCopy(vm.Object, "spec", "template", "spec", "domain", "cpu", "cores")
	if !ok {
		t.Fatal("missing cpu.cores")
	}
	if cores != int64(4) {
		t.Errorf("expected 4 cores, got %v", cores)
	}

	mem, ok, _ := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")
	if !ok || mem != "4Gi" {
		t.Errorf("expected memory 4Gi, got %q", mem)
	}
}

func TestBuildVMWithCloudInit(t *testing.T) {
	labels := map[string]any{"app": "test"}
	props := map[string]any{
		"userData": "#cloud-config\npackages: [nginx]\n",
	}
	vm := buildVM("test-vm", "default", labels, "fedora:latest", 1, "1Gi", true, props)

	volumes, ok, _ := unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	if !ok {
		t.Fatal("missing volumes")
	}
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes (rootdisk + cloudinit), got %d", len(volumes))
	}

	// Second volume should be cloudinit.
	ciVol := volumes[1].(map[string]any)
	if ciVol["name"] != "cloudinit" {
		t.Errorf("expected cloudinit volume, got %v", ciVol["name"])
	}
}
