package kubernetes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	client        kubernetes.Interface
	namespace     string
	lbWaitTimeout time.Duration // how long to wait for LoadBalancer IP (default: 60s)
}

// Config holds the configuration for the Kubernetes provider.
type Config struct {
	Kubeconfig string
	Namespace  string
	Context    string
}

// New creates a Kubernetes provider from the given config.
// If kubeconfig is empty, it uses the default loading rules (~/.kube/config).
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

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}

	return &Provider{client: client, namespace: ns, lbWaitTimeout: 60 * time.Second}, nil
}

// NewFromClient creates a provider with an existing client (useful for testing).
func NewFromClient(client kubernetes.Interface, namespace string) *Provider {
	if namespace == "" {
		namespace = "default"
	}
	return &Provider{client: client, namespace: namespace, lbWaitTimeout: 0}
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

// serviceConfig holds parsed service properties.
type serviceConfig struct {
	svcType corev1.ServiceType
	port    int32 // external service port
}

func parseServiceConfig(props map[string]interface{}, containerPort int32) serviceConfig {
	cfg := serviceConfig{
		svcType: corev1.ServiceTypeClusterIP,
		port:    containerPort,
	}

	svcProps, ok := props["service"].(map[string]interface{})
	if !ok {
		return cfg
	}

	if t, ok := svcProps["type"].(string); ok {
		switch strings.ToLower(t) {
		case "loadbalancer":
			cfg.svcType = corev1.ServiceTypeLoadBalancer
		case "nodeport":
			cfg.svcType = corev1.ServiceTypeNodePort
		default:
			cfg.svcType = corev1.ServiceTypeClusterIP
		}
	}

	if p, ok := svcProps["port"]; ok {
		cfg.port = toInt32(p)
	}

	return cfg
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

	containerPort := int32(8080)
	if pp, ok := props["port"]; ok {
		containerPort = toInt32(pp)
	}

	svcCfg := parseServiceConfig(props, containerPort)

	var envVars []corev1.EnvVar
	if envMap, ok := props["env"].(map[string]interface{}); ok {
		for k, v := range envMap {
			envVars = append(envVars, corev1.EnvVar{
				Name:  k,
				Value: fmt.Sprintf("%v", v),
			})
		}
	}

	switch diff.Action {
	case types.DiffActionCreate:
		actualName := generateK8sName(name)
		if err := p.applyResources(ctx, actualName, image, replicas, containerPort, svcCfg, envVars, true); err != nil {
			return nil, err
		}
		return p.buildOutputs(ctx, actualName, diff.Type, props, svcCfg), nil

	case types.DiffActionUpdate:
		if err := p.applyResources(ctx, name, image, replicas, containerPort, svcCfg, envVars, false); err != nil {
			return nil, err
		}
		return p.buildOutputs(ctx, name, diff.Type, props, svcCfg), nil
	}

	return &types.Resource{
		Name:       name,
		Type:       diff.Type,
		Provider:   p.Name(),
		Properties: props,
		Status:     types.ResourceStatusReady,
	}, nil
}

func (p *Provider) applyResources(ctx context.Context, name, image string, replicas, containerPort int32, svcCfg serviceConfig, envVars []corev1.EnvVar, create bool) error {
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
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: image,
						Ports: []corev1.ContainerPort{{ContainerPort: containerPort}},
						Env:   envVars,
					}},
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
			Type:     svcCfg.svcType,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       svcCfg.port,
				TargetPort: intstr.FromInt32(containerPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	deploymentsClient := p.client.AppsV1().Deployments(p.namespace)
	servicesClient := p.client.CoreV1().Services(p.namespace)

	if create {
		if _, err := deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating deployment %q: %w", name, err)
		}
		if _, err := servicesClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating service %q: %w", name, err)
		}
	} else {
		if _, err := deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating deployment %q: %w", name, err)
		}
		// Preserve existing service status (e.g. LoadBalancer ingress) during update.
		existing, err := servicesClient.Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			svc.Spec.ClusterIP = existing.Spec.ClusterIP
			svc.Status = existing.Status
			svc.ResourceVersion = existing.ResourceVersion
		}
		if _, err := servicesClient.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating service %q: %w", name, err)
		}
	}

	return nil
}

func (p *Provider) buildOutputs(ctx context.Context, name string, resType types.ResourceType, props map[string]interface{}, svcCfg serviceConfig) *types.Resource {
	internalDNS := fmt.Sprintf("%s.%s.svc.cluster.local", name, p.namespace)
	outputs := map[string]interface{}{
		"service":   internalDNS,
		"namespace": p.namespace,
		"port":      svcCfg.port,
	}

	// For LoadBalancer, try to read the assigned external IP.
	if svcCfg.svcType == corev1.ServiceTypeLoadBalancer {
		ip := p.getLoadBalancerIP(ctx, name)
		outputs["ip"] = ip
		if ip != "" {
			outputs["url"] = fmt.Sprintf("http://%s:%d", ip, svcCfg.port)
		} else {
			outputs["url"] = fmt.Sprintf("http://%s:%d", internalDNS, svcCfg.port)
		}
	} else {
		outputs["ip"] = ""
		outputs["url"] = fmt.Sprintf("http://%s:%d", internalDNS, svcCfg.port)
	}

	return &types.Resource{
		Name:       name,
		Type:       resType,
		Provider:   p.Name(),
		Properties: props,
		Outputs:    outputs,
		Status:     types.ResourceStatusReady,
	}
}

func (p *Provider) getLoadBalancerIP(ctx context.Context, name string) string {
	if p.lbWaitTimeout <= 0 {
		return p.tryGetLoadBalancerIP(ctx, name)
	}

	// LoadBalancer IPs take time to provision. Retry until timeout.
	deadline := time.After(p.lbWaitTimeout)
	for {
		if ip := p.tryGetLoadBalancerIP(ctx, name); ip != "" {
			return ip
		}
		select {
		case <-deadline:
			return ""
		case <-ctx.Done():
			return ""
		case <-time.After(5 * time.Second):
		}
	}
}

func (p *Provider) tryGetLoadBalancerIP(ctx context.Context, name string) string {
	svc, err := p.client.CoreV1().Services(p.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			return ingress.IP
		}
		if ingress.Hostname != "" {
			return ingress.Hostname
		}
	}
	return ""
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

func generateK8sName(prefix string) string {
	b := make([]byte, 4)
	rand.Read(b)
	suffix := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", prefix, suffix)
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
