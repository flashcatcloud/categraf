//go:build no_ibex

package agent

type IbexAgent struct{}

func NewIbexAgent() *IbexAgent {
	return nil
}

func (a *IbexAgent) Start() error {
	return nil
}

func (a *IbexAgent) Stop() error {
	return nil
}
