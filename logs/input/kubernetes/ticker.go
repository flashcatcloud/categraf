package kubernetes

import (
	"context"
	logService "flashcat.cloud/categraf/logs/service"
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/logs/util/kubernetes/kubelet"
)

type (
	Scanner struct {
		kubelet  kubelet.KubeUtilInterface
		services *logService.Services
	}
)

func NewScanner(services *logService.Services) *Scanner {
	return &Scanner{
		services: services,
	}
}

func (s *Scanner) Scan() {
	var (
		err error
	)
	if s.kubelet == nil {
		s.kubelet, err = kubelet.GetKubeUtil()
		if err != nil {
			log.Printf("connect kubelet error %s", err)
			return
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pods, err := s.kubelet.GetLocalPodList(ctx)
			if err != nil {
				log.Printf("get local pod list error %s", err)
				return
			}
			for _, pod := range pods {
				for _, container := range pod.Status.GetAllContainers() {
					svc := logService.NewService("docker", strings.TrimPrefix(container.ID, "docker://"), logService.After)
					s.services.AddService(svc)
				}
			}
		}
	}
}
