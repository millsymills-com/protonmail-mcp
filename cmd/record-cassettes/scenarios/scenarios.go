//go:build recording

package scenarios

import (
	"context"
	"sort"
)

// Func is a scenario entrypoint. Each scenario is responsible for setup,
// exercise, and teardown of every resource it touches.
type Func func(ctx context.Context) error

var registry = map[string]Func{}

// RegisterAll wires every scenario into the registry. Call once at startup
// before Names() or Lookup(). Each sibling file in this package contributes
// an unexported register*() that this function invokes.
func RegisterAll() {
	registerCLIFlows()
	registerCustomDomainLifecycle()
	registerErrorEnvelopes()
	registerLogoutInvalidates()
	registerReadTools()
	registerRefreshRevoked()
	registerReloginAfterRefreshReject()
	registerServerBoot()
	registerTokenRotation()
	registerWriteAddresses()
	registerWriteSettings()
}

// Register attaches fn to name. Panics on duplicate registration.
func Register(name string, fn Func) {
	if _, dup := registry[name]; dup {
		panic("duplicate scenario: " + name)
	}
	registry[name] = fn
}

func Lookup(name string) (Func, bool) {
	fn, ok := registry[name]
	return fn, ok
}

func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
