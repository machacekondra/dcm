package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/dcm-io/dcm/pkg/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultImage          = "postgres:16"
	defaultPort           = int32(5432)
	defaultStorage        = "1Gi"
	defaultMaxConnections = 100
)

// Provider manages PostgreSQL instances on Kubernetes using StatefulSets.
type Provider struct {
	client    kubernetes.Interface
	namespace string
}

// Config holds the configuration for the PostgreSQL provider.
type Config struct {
	Kubeconfig string
	Namespace  string
	Context    string
}

// New creates a PostgreSQL provider from the given config.
func New(cfg Config) (*Provider, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		overrides.CurrentContext = cfg.Context
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

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

func (p *Provider) Name() string { return "postgres" }

func (p *Provider) Capabilities() []types.ResourceType {
	return []types.ResourceType{types.ResourceTypePostgres}
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
	name := diff.Resource
	props := diff.After

	// Parse properties.
	version := getString(props, "version", "16")
	image := fmt.Sprintf("postgres:%s", version)
	if img, ok := props["image"].(string); ok && img != "" {
		image = img
	}

	storage := getString(props, "storage", defaultStorage)
	maxConns := getInt(props, "maxConnections", defaultMaxConnections)
	dbName := getString(props, "database", "app")
	dbUser := getString(props, "username", "postgres")
	dbPass := getString(props, "password", "postgres")

	labels := map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/managed-by": "dcm",
	}

	switch diff.Action {
	case types.DiffActionCreate:
		actualName := generateName(name)
		labels["app.kubernetes.io/name"] = actualName
		if err := p.createResources(ctx, actualName, labels, image, storage, maxConns, dbName, dbUser, dbPass); err != nil {
			return nil, err
		}
		return p.buildResource(actualName, diff.Type, props, dbName, dbUser, dbPass), nil

	case types.DiffActionUpdate:
		labels["app.kubernetes.io/name"] = name
		if err := p.updateResources(ctx, name, labels, image, maxConns, dbName, dbUser, dbPass); err != nil {
			return nil, err
		}
		return p.buildResource(name, diff.Type, props, dbName, dbUser, dbPass), nil
	}

	return &types.Resource{
		Name:       name,
		Type:       diff.Type,
		Provider:   p.Name(),
		Properties: props,
		Status:     types.ResourceStatusReady,
	}, nil
}

func (p *Provider) createResources(ctx context.Context, name string, labels map[string]string, image, storage string, maxConns int, dbName, dbUser, dbPass string) error {
	// Secret for credentials.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.namespace,
			Labels:    labels,
		},
		StringData: map[string]string{
			"POSTGRES_DB":       dbName,
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPass,
		},
	}
	if _, err := p.client.CoreV1().Secrets(p.namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating secret %q: %w", name, err)
	}

	// StatefulSet with PVC template.
	replicas := int32(1)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name,
			Replicas:    &replicas,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "postgres",
						Image: image,
						Ports: []corev1.ContainerPort{{
							Name:          "postgres",
							ContainerPort: defaultPort,
						}},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name},
							},
						}},
						Args: []string{
							"-c", fmt.Sprintf("max_connections=%d", maxConns),
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "data",
							MountPath: "/var/lib/postgresql/data",
							SubPath:   "pgdata",
						}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromInt32(defaultPort),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "data"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(storage),
						},
					},
				},
			}},
		},
	}
	if _, err := p.client.AppsV1().StatefulSets(p.namespace).Create(ctx, sts, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating statefulset %q: %w", name, err)
	}

	// Headless service for stable DNS.
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{{
				Name:       "postgres",
				Port:       defaultPort,
				TargetPort: intstr.FromInt32(defaultPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	if _, err := p.client.CoreV1().Services(p.namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating service %q: %w", name, err)
	}

	return nil
}

func (p *Provider) updateResources(ctx context.Context, name string, labels map[string]string, image string, maxConns int, dbName, dbUser, dbPass string) error {
	// Update secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.namespace,
			Labels:    labels,
		},
		StringData: map[string]string{
			"POSTGRES_DB":       dbName,
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPass,
		},
	}
	if _, err := p.client.CoreV1().Secrets(p.namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating secret %q: %w", name, err)
	}

	// Update StatefulSet (image, args).
	sts, err := p.client.AppsV1().StatefulSets(p.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting statefulset %q: %w", name, err)
	}
	sts.Spec.Template.Spec.Containers[0].Image = image
	sts.Spec.Template.Spec.Containers[0].Args = []string{
		"-c", fmt.Sprintf("max_connections=%d", maxConns),
	}
	if _, err := p.client.AppsV1().StatefulSets(p.namespace).Update(ctx, sts, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating statefulset %q: %w", name, err)
	}

	return nil
}

func (p *Provider) Destroy(res *types.Resource) error {
	ctx := context.Background()
	name := res.Name

	// Delete StatefulSet.
	err := p.client.AppsV1().StatefulSets(p.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting statefulset %q: %w", name, err)
	}

	// Delete Service.
	err = p.client.CoreV1().Services(p.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting service %q: %w", name, err)
	}

	// Delete Secret.
	err = p.client.CoreV1().Secrets(p.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting secret %q: %w", name, err)
	}

	return nil
}

func (p *Provider) Status(res *types.Resource) (types.ResourceStatus, error) {
	ctx := context.Background()

	sts, err := p.client.AppsV1().StatefulSets(p.namespace).Get(ctx, res.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return types.ResourceStatusUnknown, nil
		}
		return types.ResourceStatusFailed, fmt.Errorf("getting statefulset %q: %w", res.Name, err)
	}

	if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
		return types.ResourceStatusReady, nil
	}
	if sts.Status.ReadyReplicas > 0 {
		return types.ResourceStatusUpdating, nil
	}
	return types.ResourceStatusCreating, nil
}

func (p *Provider) buildResource(name string, resType types.ResourceType, props map[string]interface{}, dbName, dbUser, dbPass string) *types.Resource {
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", name, name, p.namespace)
	connStr := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", dbUser, dbPass, host, defaultPort, dbName)

	return &types.Resource{
		Name:       name,
		Type:       resType,
		Provider:   p.Name(),
		Properties: props,
		Outputs: map[string]interface{}{
			"host":             host,
			"port":             defaultPort,
			"database":         dbName,
			"username":         dbUser,
			"connectionString": connStr,
			"service":          fmt.Sprintf("%s.%s.svc.cluster.local", name, p.namespace),
			"namespace":        p.namespace,
		},
		Status: types.ResourceStatusReady,
	}
}

func getString(props map[string]interface{}, key, fallback string) string {
	if v, ok := props[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func getInt(props map[string]interface{}, key string, fallback int) int {
	switch v := props[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func generateName(prefix string) string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
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
