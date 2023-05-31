package main

import (
	"context"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

type dnsResolver struct {
	adminDomain     string
	adminIP         net.IP
	domainOverrides map[string]net.IP
}

func newDNSResolver(adminDomain string, domainOverrides map[string]net.IP) dnsResolver {
	if domainOverrides == nil {
		domainOverrides = make(map[string]net.IP)
	}
	return dnsResolver{
		adminDomain:     adminDomain,
		adminIP:         net.ParseIP("127.0.0.1"),
		domainOverrides: domainOverrides,
	}
}

// Resolve implement interface NameResolver
func (d dnsResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	if name == d.adminDomain {
		return ctx, d.adminIP, nil
	}
	overrideIP, found := d.domainOverrides[name]
	if found {
		return ctx, overrideIP, nil
	}
	addr, err := net.ResolveIPAddr("ip", name)
	if err != nil {
		return ctx, nil, err
	}
	return ctx, addr.IP, err
}

type dnsOverride struct {
	FQDN string
	IP   string
}

func dnsOverridesFromFile(fname string) (map[string]net.IP, error) {
	contents, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	var overrides []dnsOverride
	if err := yaml.Unmarshal(contents, &overrides); err != nil {
		return nil, err
	}
	result := make(map[string]net.IP)
	for _, ov := range overrides {
		if ov.FQDN == "" {
			continue
		}
		result[ov.FQDN] = net.ParseIP(ov.IP)
	}
	return result, nil
}
