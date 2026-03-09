package ipam

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"

	"github.com/dcm-io/dcm/pkg/types"
)

// StaticProvider is a simple pool-based IPAM provider.
// It allocates IPs from configured subnet ranges without an external system.
type StaticProvider struct {
	mu        sync.Mutex
	pools     map[string]*pool
	allocated map[string]string // resource name -> allocated IP
}

type pool struct {
	name   string
	subnet *net.IPNet
	used   map[string]bool // IP string -> in use
}

// StaticConfig holds the configuration for the static IPAM provider.
type StaticConfig struct {
	// Pools maps pool names to CIDR ranges, e.g. {"production": "10.0.1.0/24"}.
	Pools map[string]string
}

// NewStatic creates a static IPAM provider from the given config.
func NewStatic(cfg StaticConfig) (*StaticProvider, error) {
	p := &StaticProvider{
		pools:     make(map[string]*pool),
		allocated: make(map[string]string),
	}

	for name, cidr := range cfg.Pools {
		_, subnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("pool %q: invalid CIDR %q: %w", name, cidr, err)
		}
		p.pools[name] = &pool{
			name:   name,
			subnet: subnet,
			used:   make(map[string]bool),
		}
	}

	return p, nil
}

func (p *StaticProvider) Name() string {
	return "static-ipam"
}

func (p *StaticProvider) Capabilities() []types.ResourceType {
	return []types.ResourceType{types.ResourceTypeIP}
}

func (p *StaticProvider) Plan(desired, current *types.Resource) (*types.Diff, error) {
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

func (p *StaticProvider) Apply(diff *types.Diff) (*types.Resource, error) {
	props := diff.After
	poolName, _ := props["pool"].(string)
	if poolName == "" {
		return nil, fmt.Errorf("ip resource %q requires a 'pool' property", diff.Resource)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	pl, ok := p.pools[poolName]
	if !ok {
		available := make([]string, 0, len(p.pools))
		for name := range p.pools {
			available = append(available, name)
		}
		return nil, fmt.Errorf("unknown IPAM pool %q (available: %v)", poolName, available)
	}

	switch diff.Action {
	case types.DiffActionCreate:
		ip, err := pl.allocate()
		if err != nil {
			return nil, fmt.Errorf("allocating IP from pool %q: %w", poolName, err)
		}
		p.allocated[diff.Resource] = ip

		ones, _ := pl.subnet.Mask.Size()
		return &types.Resource{
			Name:       diff.Resource,
			Type:       diff.Type,
			Provider:   p.Name(),
			Properties: props,
			Outputs: map[string]any{
				"address": ip,
				"cidr":    fmt.Sprintf("%s/%d", ip, ones),
				"pool":    poolName,
			},
			Status: types.ResourceStatusReady,
		}, nil

	case types.DiffActionUpdate:
		// Keep existing allocation, just update properties.
		ip := p.allocated[diff.Resource]
		if ip == "" {
			// Re-allocate if somehow lost.
			var err error
			ip, err = pl.allocate()
			if err != nil {
				return nil, fmt.Errorf("allocating IP from pool %q: %w", poolName, err)
			}
			p.allocated[diff.Resource] = ip
		}

		ones, _ := pl.subnet.Mask.Size()
		return &types.Resource{
			Name:       diff.Resource,
			Type:       diff.Type,
			Provider:   p.Name(),
			Properties: props,
			Outputs: map[string]any{
				"address": ip,
				"cidr":    fmt.Sprintf("%s/%d", ip, ones),
				"pool":    poolName,
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

func (p *StaticProvider) Destroy(resource *types.Resource) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ip := p.allocated[resource.Name]
	if ip != "" {
		// Release from all pools.
		for _, pl := range p.pools {
			delete(pl.used, ip)
		}
		delete(p.allocated, resource.Name)
	}
	return nil
}

func (p *StaticProvider) Status(resource *types.Resource) (types.ResourceStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.allocated[resource.Name]; ok {
		return types.ResourceStatusReady, nil
	}
	return types.ResourceStatusUnknown, nil
}

// allocate picks a random available IP from the pool's subnet.
func (pl *pool) allocate() (string, error) {
	ones, bits := pl.subnet.Mask.Size()
	hostBits := bits - ones
	if hostBits <= 2 {
		return "", fmt.Errorf("subnet %s too small", pl.subnet)
	}

	// Total usable hosts (excluding network and broadcast).
	totalHosts := (1 << hostBits) - 2

	// Try random allocation with retries.
	networkIP := binary.BigEndian.Uint32(pl.subnet.IP.To4())

	for range min(totalHosts, 1000) {
		// Random host number from 1 to totalHosts (skip .0 network addr).
		hostNum := uint32(rand.IntN(totalHosts)) + 1
		ipNum := networkIP + hostNum
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, ipNum)
		ipStr := ip.String()

		if !pl.used[ipStr] {
			pl.used[ipStr] = true
			return ipStr, nil
		}
	}

	return "", fmt.Errorf("no available IPs in pool %s (%d/%d used)", pl.name, len(pl.used), totalHosts)
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
