package agent

import (
	"context"
	"log"

	"flashcat.cloud/categraf/api"
	"flashcat.cloud/categraf/config"
)

func (a *Agent) startHttpAgent() (err error) {
	if config.Config.HTTPServer == nil || !config.Config.HTTPServer.Enable {
		return nil
	}

	defer func() {
		if err != nil {
			log.Println("E! failed to start http agent:", err)
		}
	}()

	server := api.NewServer(api.Address(config.Config.HTTPServer.Address))
	err = server.Start(context.TODO())
	if err != nil {
		return err
	}

	a.Server = server
	return nil
}

func (a *Agent) stopHttpAgent() (err error) {
	if config.Config.HTTPServer == nil || !config.Config.HTTPServer.Enable || a.Server == nil {
		return nil
	}

	defer func() {
		if err != nil {
			log.Println("E! failed to stop http agent:", err)
		}
	}()

	return a.Server.Stop(context.TODO())
}
