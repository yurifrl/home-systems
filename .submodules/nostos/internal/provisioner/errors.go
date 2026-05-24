package provisioner

import "errors"

// Sentinel errors returned across the provisioner subsystem. Tests and
// callers use errors.Is for matching.
var (
	// ErrPreflight wraps any failure during the Preflight hook that
	// should abort the install before side effects.
	ErrPreflight = errors.New("provisioner: preflight failed")

	// ErrBoot wraps a Boot-phase failure (e.g. tpi flash failed).
	ErrBoot = errors.New("provisioner: boot failed")

	// ErrTimeout is returned by hooks whose deadline expires.
	ErrTimeout = errors.New("provisioner: timeout")

	// ErrLocked is returned by the per-node flock acquirer when another
	// process already holds the node lock.
	ErrLocked = errors.New("provisioner: node locked")

	// ErrNodeAlreadyReady is returned by the live-node reinstall guard
	// when a healthy cluster member is targeted without --reinstall.
	ErrNodeAlreadyReady = errors.New("provisioner: node already ready")

	// ErrNotRegistered is returned by New when the requested method has
	// no registered factory.
	ErrNotRegistered = errors.New("provisioner: method not registered")
)
