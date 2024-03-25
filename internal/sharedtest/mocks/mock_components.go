package mocks

import "github.com/launchdarkly/go-server-sdk/v6/subsystems"

// SingleComponentConfigurer is a test implementation of ComponentConfigurer that always returns the same
// pre-existing instance.
type SingleComponentConfigurer[T any] struct {
	Instance T
}

// Build builds the component.
func (c SingleComponentConfigurer[T]) Build(clientContext subsystems.ClientContext) (T, error) {
	return c.Instance, nil
}

// ComponentConfigurerThatReturnsError is a test implementation of ComponentConfigurer that always returns
// an error.
type ComponentConfigurerThatReturnsError[T any] struct {
	Err error
}

// Build builds the component.
func (c ComponentConfigurerThatReturnsError[T]) Build(clientContext subsystems.ClientContext) (T, error) {
	var empty T
	return empty, c.Err
}

// ComponentConfigurerThatCapturesClientContext is a test decorator for a ComponentConfigurer that allows
// tests to see the ClientContext that was passed to it.
type ComponentConfigurerThatCapturesClientContext[T any] struct {
	Configurer            subsystems.ComponentConfigurer[T]
	ReceivedClientContext subsystems.ClientContext
}

// Build builds the component.
func (c *ComponentConfigurerThatCapturesClientContext[T]) Build(clientContext subsystems.ClientContext) (T, error) {
	c.ReceivedClientContext = clientContext
	return c.Configurer.Build(clientContext)
}
