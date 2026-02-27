package testkit

import (
	"context"
	"net"
)

// FakeResolver returns preconfigured results.
type FakeResolver struct {
	IPs []string
	Err error
}

func (f *FakeResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	return f.IPs, f.Err
}

// FakeDialer returns preconfigured results.
type FakeDialer struct {
	Conn net.Conn
	Err  error
}

func (f *FakeDialer) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	return f.Conn, f.Err
}
