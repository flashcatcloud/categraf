//go:build no_ibex

package agent

type IbexAgent struct{}

func NewIbexAgent() AgentModule {
	return nil
}

func (a *IbexAgent) Start() error {
	return nil
}

func (a *IbexAgent) Stop() error {
	return nil
}
