package kubernetes

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dcm-io/dcm/pkg/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Provider manages container resources on Kubernetes by creating
// Deployments and Services.
type Provider struct {
	client    kubernetes.Interface
	namespace string
}

// Config holds the configuration for the Kubernetes provider.
type Config struct {
	Kubeconfig string
	Namespace  string
}

// New creates a Kubernetes provider from the given config.
// If kubeconfig is empty, it uses the default loading rules (~/.kube/config).
func New(cfg Config) (*Provider, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building kubernetes config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}

	return &Provider{client: client, namespace: ns}, nil
}

// NewFromClient creates a provider with an existing client (useful for testing).
func NewFromClient(client kubernetes.Interface, namespace string) *Provider {
	if namespace == "" {
		namespace = "default"
	}
	return &Provider{client: client, namespace: namespace}
}

func (p *Provider) Name() string {
	return "kubernetes"
}

func (p *Provider) Capabilities() []types.ResourceType {
	return []types.ResourceType{
		types.ResourceTypeContainer,
	}
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
			Resource: desired.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			Before:   current.Properties,
			After:    desired.Properties,
		}, nil
	}

	return &types.Diff{
		Action:   types.DiffActionNone,
		Resource: desired.Name,
		Type:     desired.Type,
		Provider: p.Name(),
	}, nil
}

func (p *Provider) Apply(diff *types.Diff) (*types.Resource, error) {
	ctx := context.Background()
	name := diff.Resource
	props := diff.After

	image, _ := props["image"].(string)
	if image == "" {
		return nil, fmt.Errorf("container resource %q requires an 'image' property", name)
	}

	replicas := int32(1)
	if r, ok := props["replicas"]; ok {
		replicas = toInt32(r)
	}

	port := int32(8080)
	if p, ok := props["port"]; ok {
		port = toInt32(p)
	}

	var envVars []corev1.EnvVar
	if envMap, ok := props["env"].(map[string]interface{}); ok {
		for k, v := range envMap {
			envVars = append(envVars, corev1.EnvVar{
				Name:  k,
				Value: fmt.Sprintf("%v", v),
			})
		}
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/managed-by": "dcm",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: image,
							Ports: []corev1.ContainerPort{
								{ContainerPort: port},
							},
							Env: envVars,
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	deploymentsClient := p.client.AppsV1().Deployments(p.namespace)
	servicesClient := p.client.CoreV1().Services(p.namespace)

	switch diff.Action {
	case types.DiffActionCreate:
		if _, err := deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("creating deployment %q: %w", name, err)
		}
		if _, err := servicesClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("creating service %q: %w", name, err)
		}

	case types.DiffActionUpdate:
		if _, err := deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return nil, fmt.Errorf("updating deployment %q: %w", name, err)
		}
		if _, err := servicesClient.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
			return nil, fmt.Errorf("updating service %q: %w", name, err)
		}
	}

	return &types.Resource{
		Name:       name,
		Type:       diff.Type,
		Provider:   p.Name(),
		Properties: props,
		Outputs: map[string]interface{}{
			"url":       fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", name, p.namespace, port),
			"service":   fmt.Sprintf("%s.%s.svc.cluster.local", name, p.namespace),
			"namespace": p.namespace,
			"port":      port,
		},
		Status: types.ResourceStatusReady,
	}, nil
}

func (p *Provider) Destroy(resource *types.Resource) error {
	ctx := context.Background()
	name := resource.Name

	err := p.client.AppsV1().Deployments(p.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting deployment %q: %w", name, err)
	}

	err = p.client.CoreV1().Services(p.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting service %q: %w", name, err)
	}

	return nil
}

func (p *Provider) Status(resource *types.Resource) (types.ResourceStatus, error) {
	ctx := context.Background()

	deploy, err := p.client.AppsV1().Deployments(p.namespace).Get(ctx, resource.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return types.ResourceStatusUnknown, nil
		}
		return types.ResourceStatusFailed, fmt.Errorf("getting deployment %q: %w", resource.Name, err)
	}

	if deploy.Status.ReadyReplicas == *deploy.Spec.Replicas {
		return types.ResourceStatusReady, nil
	}
	if deploy.Status.ReadyReplicas > 0 {
		return types.ResourceStatusUpdating, nil
	}
	return types.ResourceStatusCreating, nil
}

func toInt32(v interface{}) int32 {
	switch val := v.(type) {
	case int:
		return int32(val)
	case int32:
		return val
	case int64:
		return int32(val)
	case float64:
		return int32(val)
	case string:
		n, _ := strconv.Atoi(val)
		return int32(n)
	default:
		return 1
	}
}

func propsEqual(a, b map[string]interface{}) bool {
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
