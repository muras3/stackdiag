package testkit

import (
	"context"
	"net"
)

// Resolver is the interface for DNS resolution (matches net.Resolver.LookupHost signature).
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// FakeResolver returns preconfigured results.
type FakeResolver struct {
	IPs []string
	Err error
}

func (f *FakeResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	return f.IPs, f.Err
}

// Dialer is the interface for TCP connections.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// FakeDialer returns preconfigured results.
type FakeDialer struct {
	Conn net.Conn
	Err  error
}

func (f *FakeDialer) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	return f.Conn, f.Err
}
