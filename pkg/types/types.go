package types

// Executor defines the interface for executing commands
type Executor interface {
	ExecuteCommand(name string, arg ...string) error
}
