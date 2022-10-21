package sharedtest

import "github.com/launchdarkly/go-server-sdk/v6/subsystems"

// SingleComponentConfigurer is a test implementation of ComponentConfigurer that always returns the same
// pre-existing instance.
type SingleComponentConfigurer[T any] struct {
	Instance T
}

func (c SingleComponentConfigurer[T]) Build(context subsystems.ClientContext) (T, error) {
	return c.Instance, nil
}

// ComponentConfigurerThatReturnsError is a test implementation of ComponentConfigurer that always returns
// an error.
type ComponentConfigurerThatReturnsError[T any] struct {
	Err error
}

func (c ComponentConfigurerThatReturnsError[T]) Build(context subsystems.ClientContext) (T, error) {
	var empty T
	return empty, c.Err
}
