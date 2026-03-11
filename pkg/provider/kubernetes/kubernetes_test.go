package kubernetes

import (
	"context"
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestProvider() *Provider {
	return NewFromClient(fake.NewSimpleClientset(), "test-ns")
}

func TestProvider_NameAndCapabilities(t *testing.T) {
	p := newTestProvider()

	if p.Name() != "kubernetes" {
		t.Errorf("expected name 'kubernetes', got %q", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != types.ResourceTypeContainer {
		t.Errorf("expected [container], got %v", caps)
	}
}

func TestProvider_PlanCreate(t *testing.T) {
	p := newTestProvider()

	desired := &types.Resource{
		Name: "web",
		Type: types.ResourceTypeContainer,
		Properties: map[string]interface{}{
			"image":    "nginx:latest",
			"replicas": 2,
		},
	}

	diff, err := p.Plan(desired, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionCreate {
		t.Errorf("expected create action, got %s", diff.Action)
	}
}

func TestProvider_PlanNoChange(t *testing.T) {
	p := newTestProvider()

	resource := &types.Resource{
		Name: "web",
		Type: types.ResourceTypeContainer,
		Properties: map[string]interface{}{
			"image": "nginx:latest",
		},
	}

	diff, err := p.Plan(resource, resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionNone {
		t.Errorf("expected no-op, got %s", diff.Action)
	}
}

func TestProvider_ApplyAndDestroy(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image":    "nginx:latest",
			"replicas": 3,
			"port":     8080,
			"env": map[string]interface{}{
				"APP_ENV": "production",
			},
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if resource.Status != types.ResourceStatusReady {
		t.Errorf("expected ready status, got %s", resource.Status)
	}
	if !strings.HasPrefix(resource.Name, "web-") {
		t.Errorf("expected resource name to start with 'web-', got %q", resource.Name)
	}
	if resource.Outputs["namespace"] != "test-ns" {
		t.Errorf("expected namespace test-ns in outputs, got %v", resource.Outputs["namespace"])
	}
	if resource.Outputs["ip"] != "" {
		t.Errorf("expected empty ip for ClusterIP service, got %v", resource.Outputs["ip"])
	}

	// Verify status reports correctly (fake client won't have ready replicas).
	status, err := p.Status(resource)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status != types.ResourceStatusCreating {
		t.Errorf("expected creating status from fake client, got %s", status)
	}

	// Destroy should succeed.
	if err := p.Destroy(resource); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	// Status after destroy should be unknown.
	status, err = p.Status(resource)
	if err != nil {
		t.Fatalf("status after destroy failed: %v", err)
	}
	if status != types.ResourceStatusUnknown {
		t.Errorf("expected unknown status after destroy, got %s", status)
	}
}

func TestProvider_ApplyMissingImage(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After:    map[string]interface{}{},
	}

	_, err := p.Apply(diff)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestProvider_ApplyLoadBalancerService(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image": "nginx:latest",
			"port":  8080,
			"service": map[string]interface{}{
				"type": "LoadBalancer",
				"port": 80,
			},
		},
	}

	res, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Verify service port is 80, not 8080.
	if res.Outputs["port"] != int32(80) {
		t.Errorf("expected service port 80, got %v", res.Outputs["port"])
	}

	// Verify service was created with LoadBalancer type.
	svc, err := p.client.CoreV1().Services(p.namespace).Get(context.Background(), res.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting service: %v", err)
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Errorf("expected LoadBalancer type, got %s", svc.Spec.Type)
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("expected service port 80, got %d", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntVal != 8080 {
		t.Errorf("expected target port 8080, got %d", svc.Spec.Ports[0].TargetPort.IntVal)
	}

	// No LB IP assigned yet (fake client), ip should be empty.
	if res.Outputs["ip"] != "" {
		t.Errorf("expected empty ip before LB assignment, got %v", res.Outputs["ip"])
	}
	// URL should fall back to internal DNS.
	url, _ := res.Outputs["url"].(string)
	if !strings.Contains(url, "svc.cluster.local") {
		t.Errorf("expected internal DNS fallback in url, got %q", url)
	}
}

func TestProvider_ApplyLoadBalancerWithIP(t *testing.T) {
	client := fake.NewSimpleClientset()
	p := NewFromClient(client, "test-ns")

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image": "nginx:latest",
			"port":  8080,
			"service": map[string]interface{}{
				"type": "LoadBalancer",
				"port": 80,
			},
		},
	}

	res, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Simulate LB IP assignment by updating the service status.
	ctx := context.Background()
	svc, _ := client.CoreV1().Services("test-ns").Get(ctx, res.Name, metav1.GetOptions{})
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "192.168.1.100"}}
	_, err = client.CoreV1().Services("test-ns").UpdateStatus(ctx, svc, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("updating service status: %v", err)
	}

	// Now do an update to re-read the IP.
	updateDiff := &types.Diff{
		Action:   types.DiffActionUpdate,
		Resource: res.Name,
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image": "nginx:latest",
			"port":  8080,
			"service": map[string]interface{}{
				"type": "LoadBalancer",
				"port": 80,
			},
		},
	}

	updated, err := p.Apply(updateDiff)
	if err != nil {
		t.Fatalf("update apply failed: %v", err)
	}

	if updated.Outputs["ip"] != "192.168.1.100" {
		t.Errorf("expected ip=192.168.1.100, got %v", updated.Outputs["ip"])
	}
	url, _ := updated.Outputs["url"].(string)
	if url != "http://192.168.1.100:80" {
		t.Errorf("expected url with LB IP, got %q", url)
	}
}

func TestProvider_ServiceCustomPort(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "api",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image": "myapp:latest",
			"port":  3000,
			"service": map[string]interface{}{
				"port": 443,
			},
		},
	}

	res, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if res.Outputs["port"] != int32(443) {
		t.Errorf("expected service port 443, got %v", res.Outputs["port"])
	}

	// Verify k8s service has correct port mapping.
	svc, err := p.client.CoreV1().Services(p.namespace).Get(context.Background(), res.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting service: %v", err)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("expected ClusterIP (default), got %s", svc.Spec.Type)
	}
	if svc.Spec.Ports[0].Port != 443 {
		t.Errorf("expected service port 443, got %d", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntVal != 3000 {
		t.Errorf("expected target port 3000, got %d", svc.Spec.Ports[0].TargetPort.IntVal)
	}
}

func TestProvider_DefaultServiceConfig(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image": "nginx:latest",
			"port":  8080,
		},
	}

	res, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Without service property, should default to ClusterIP with container port.
	if res.Outputs["port"] != int32(8080) {
		t.Errorf("expected default port 8080, got %v", res.Outputs["port"])
	}

	svc, err := p.client.CoreV1().Services(p.namespace).Get(context.Background(), res.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting service: %v", err)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("expected ClusterIP, got %s", svc.Spec.Type)
	}
}
