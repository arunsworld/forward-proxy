package main

import (
	"context"
	"net"
)

type dnsResolver struct {
	adminDomain string
	adminIP     net.IP
}

func newDNSResolver(adminDomain string) dnsResolver {
	return dnsResolver{
		adminDomain: adminDomain,
		adminIP:     net.ParseIP("127.0.0.1"),
	}
}

// Resolve implement interface NameResolver
func (d dnsResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	if name == d.adminDomain {
		return ctx, d.adminIP, nil
	}
	addr, err := net.ResolveIPAddr("ip", name)
	if err != nil {
		return ctx, nil, err
	}
	return ctx, addr.IP, err
}
