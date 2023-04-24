package api

import (
	"crypto/tls"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/aop"
)

func Start() {
	if config.Config == nil ||
		config.Config.HTTP == nil ||
		!config.Config.HTTP.Enable {
		return
	}
	// test mode 不开启
	if config.Config.TestMode {
		return
	}

	conf := config.Config.HTTP

	gin.SetMode(conf.RunMode)

	if strings.ToLower(conf.RunMode) == "release" {
		aop.DisableConsoleColor()
	}

	r := gin.New()
	r.Use(aop.Recovery())

	if conf.PrintAccess {
		r.Use(aop.Logger())
	}

	configRoutes(r)

	srv := &http.Server{
		Addr:         conf.Address,
		Handler:      r,
		ReadTimeout:  time.Duration(conf.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(conf.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(conf.IdleTimeout) * time.Second,
	}

	log.Println("I! http server listening on:", conf.Address)

	var err error
	if conf.CertFile != "" && conf.KeyFile != "" {
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		err = srv.ListenAndServeTLS(conf.CertFile, conf.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

func configRoutes(r *gin.Engine) {
	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	g := r.Group("/api/push")
	g.POST("/opentsdb", openTSDB)
	g.POST("/openfalcon", openFalcon)
	g.POST("/remotewrite", remoteWrite)
	g.POST("/pushgateway", pushgateway)
}
