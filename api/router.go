package api

import (
	"github.com/gin-gonic/gin"
)

type Router struct {
	*gin.Engine
}

func newRouter(srv *Server) *Router {
	r := &Router{
		Engine: srv.engine,
	}

	r.push()
	return r
}

func (r *Router) push() {
	p := push{}
	g := r.Group("/api/push")
	g.POST("/opentsdb", p.OpenTSDB)      // 发送OpenTSDB数据
	g.POST("/openfalcon", p.falcon)      // 发送OpenFalcon数据
	g.POST("/prometheus", p.remoteWrite) // 发送Prometheus数据
}
