package kubevirt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/dcm-io/dcm/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var vmGVR = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "virtualmachines",
}

var vmiGVR = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "virtualmachineinstances",
}

// Provider manages VM resources on KubeVirt.
type Provider struct {
	client    dynamic.Interface
	namespace string
}

// Config holds the configuration for the KubeVirt provider.
type Config struct {
	Kubeconfig string
	Namespace  string
	Context    string
}

// New creates a KubeVirt provider from the given config.
func New(cfg Config) (*Provider, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		configOverrides.CurrentContext = cfg.Context
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building kubernetes config: %w", err)
	}

	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}

	return &Provider{client: client, namespace: ns}, nil
}

// NewFromClient creates a provider with an existing dynamic client (useful for testing).
func NewFromClient(client dynamic.Interface, namespace string) *Provider {
	if namespace == "" {
		namespace = "default"
	}
	return &Provider{client: client, namespace: namespace}
}

func (p *Provider) Name() string {
	return "kubevirt"
}

func (p *Provider) Capabilities() []types.ResourceType {
	return []types.ResourceType{types.ResourceTypeVM}
}

func (p *Provider) Plan(desired, current *types.Resource) (*types.Diff, error) {
	if current == nil {
		return &types.Diff{
			Action:   types.DiffActionCreate,
			Resource: desired.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			After:    desired.Properties,
		}, nil
	}

	if !propsEqual(desired.Properties, current.Properties) {
		return &types.Diff{
			Action:   types.DiffActionUpdate,
			Resource: current.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			Before:   current.Properties,
			After:    desired.Properties,
		}, nil
	}

	return &types.Diff{
		Action:   types.DiffActionNone,
		Resource: current.Name,
		Type:     desired.Type,
		Provider: p.Name(),
	}, nil
}

func (p *Provider) Apply(diff *types.Diff) (*types.Resource, error) {
	ctx := context.Background()
	props := diff.After

	image := getString(props, "image")
	if image == "" {
		return nil, fmt.Errorf("vm resource %q requires an 'image' property", diff.Resource)
	}

	cpu := getInt(props, "cpu", 1)
	memory := getString(props, "memory")
	if memory == "" {
		memory = fmt.Sprintf("%dMi", getInt(props, "memoryMB", 1024))
	}

	running := true

	vmClient := p.client.Resource(vmGVR).Namespace(p.namespace)

	switch diff.Action {
	case types.DiffActionCreate:
		name := generateName(diff.Resource)
		labels := map[string]any{
			"app.kubernetes.io/name":       name,
			"app.kubernetes.io/managed-by": "dcm",
		}

		vm := buildVM(name, p.namespace, labels, image, cpu, memory, running, props)

		if _, err := vmClient.Create(ctx, vm, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("creating VirtualMachine %q: %w", name, err)
		}

		return &types.Resource{
			Name:       name,
			Type:       diff.Type,
			Provider:   p.Name(),
			Properties: props,
			Outputs: map[string]any{
				"vmName":    name,
				"namespace": p.namespace,
			},
			Status: types.ResourceStatusReady,
		}, nil

	case types.DiffActionUpdate:
		name := diff.Resource
		labels := map[string]any{
			"app.kubernetes.io/name":       name,
			"app.kubernetes.io/managed-by": "dcm",
		}

		vm := buildVM(name, p.namespace, labels, image, cpu, memory, running, props)

		if _, err := vmClient.Update(ctx, vm, metav1.UpdateOptions{}); err != nil {
			return nil, fmt.Errorf("updating VirtualMachine %q: %w", name, err)
		}

		return &types.Resource{
			Name:       name,
			Type:       diff.Type,
			Provider:   p.Name(),
			Properties: props,
			Outputs: map[string]any{
				"vmName":    name,
				"namespace": p.namespace,
			},
			Status: types.ResourceStatusReady,
		}, nil
	}

	return &types.Resource{
		Name:       diff.Resource,
		Type:       diff.Type,
		Provider:   p.Name(),
		Properties: props,
		Status:     types.ResourceStatusReady,
	}, nil
}

func (p *Provider) Destroy(resource *types.Resource) error {
	ctx := context.Background()
	err := p.client.Resource(vmGVR).Namespace(p.namespace).Delete(ctx, resource.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting VirtualMachine %q: %w", resource.Name, err)
	}
	return nil
}

func (p *Provider) Status(resource *types.Resource) (types.ResourceStatus, error) {
	ctx := context.Background()

	// Check VMI (running instance) status.
	vmi, err := p.client.Resource(vmiGVR).Namespace(p.namespace).Get(ctx, resource.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// VMI doesn't exist — check if VM exists.
			_, vmErr := p.client.Resource(vmGVR).Namespace(p.namespace).Get(ctx, resource.Name, metav1.GetOptions{})
			if vmErr != nil {
				return types.ResourceStatusUnknown, nil
			}
			return types.ResourceStatusCreating, nil
		}
		return types.ResourceStatusFailed, fmt.Errorf("getting VMI %q: %w", resource.Name, err)
	}

	phase, _, _ := unstructured.NestedString(vmi.Object, "status", "phase")
	switch phase {
	case "Running":
		return types.ResourceStatusReady, nil
	case "Scheduling", "Scheduled", "Pending":
		return types.ResourceStatusCreating, nil
	case "Failed":
		return types.ResourceStatusFailed, nil
	default:
		return types.ResourceStatusUpdating, nil
	}
}

// buildVM constructs a KubeVirt VirtualMachine unstructured object.
func buildVM(name, namespace string, labels map[string]any, image string, cpu int, memory string, running bool, props map[string]any) *unstructured.Unstructured {
	disks := []any{
		map[string]any{
			"name": "rootdisk",
			"disk": map[string]any{"bus": "virtio"},
		},
	}
	volumes := []any{
		map[string]any{
			"name": "rootdisk",
			"containerDisk": map[string]any{
				"image": image,
			},
		},
	}

	// Add cloud-init volume if userData is provided.
	if userData, ok := props["userData"].(string); ok && userData != "" {
		disks = append(disks, map[string]any{
			"name": "cloudinit",
			"disk": map[string]any{"bus": "virtio"},
		})
		volumes = append(volumes, map[string]any{
			"name": "cloudinit",
			"cloudInitNoCloud": map[string]any{
				"userData": userData,
			},
		})
	}

	// SSH key via cloudInitNoCloud if no userData but sshKey is set.
	if _, hasUD := props["userData"]; !hasUD {
		if sshKey, ok := props["sshKey"].(string); ok && sshKey != "" {
			disks = append(disks, map[string]any{
				"name": "cloudinit",
				"disk": map[string]any{"bus": "virtio"},
			})
			volumes = append(volumes, map[string]any{
				"name": "cloudinit",
				"cloudInitNoCloud": map[string]any{
					"userData": fmt.Sprintf("#cloud-config\nssh_authorized_keys:\n  - %s\n", sshKey),
				},
			})
		}
	}

	interfaces := []any{
		map[string]any{
			"name":       "default",
			"masquerade": map[string]any{},
		},
	}
	networks := []any{
		map[string]any{
			"name": "default",
			"pod":  map[string]any{},
		},
	}

	// Use a named network if specified.
	if netName, ok := props["network"].(string); ok && netName != "" {
		interfaces = []any{
			map[string]any{
				"name":   netName,
				"bridge": map[string]any{},
			},
		}
		networks = []any{
			map[string]any{
				"name": netName,
				"multus": map[string]any{
					"networkName": netName,
				},
			},
		}
	}

	vm := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"labels":    labels,
			},
			"spec": map[string]any{
				"running": running,
				"template": map[string]any{
					"metadata": map[string]any{
						"labels": labels,
					},
					"spec": map[string]any{
						"domain": map[string]any{
							"cpu": map[string]any{
								"cores": int64(cpu),
							},
							"resources": map[string]any{
								"requests": map[string]any{
									"memory": memory,
								},
							},
							"devices": map[string]any{
								"disks":      disks,
								"interfaces": interfaces,
							},
						},
						"volumes":  volumes,
						"networks": networks,
					},
				},
			},
		},
	}

	return vm
}

func generateName(prefix string) string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

func getString(props map[string]any, key string) string {
	v, _ := props[key].(string)
	return v
}

func getInt(props map[string]any, key string, defaultVal int) int {
	switch v := props[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return defaultVal
		}
		return n
	default:
		return defaultVal
	}
}

func propsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if fmt.Sprintf("%v", b[k]) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}
