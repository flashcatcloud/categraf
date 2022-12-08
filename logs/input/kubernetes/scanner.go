//go:build !no_logs

package kubernetes

import (
	"context"
	logService "flashcat.cloud/categraf/logs/service"
	"log"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/logs/util/kubernetes/kubelet"
)

type (
	Scanner struct {
		kubelet  kubelet.KubeUtilInterface
		services *logService.Services
		actives  map[string]struct{}
		mux      sync.Mutex
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
			fetched := make(map[string]struct{})
			for _, pod := range pods {
				for _, container := range pod.Status.GetAllContainers() {
					fetched[container.ID] = struct{}{}
				}
			}
			for id := range fetched {
				if !s.Contains(id) {
					svc := logService.NewService("docker", strings.TrimPrefix(id, "docker://"), logService.After)
					s.services.AddService(svc)
				}
			}

			old := s.actives
			s.SetActives(fetched)
			for id := range old {
				if !s.Contains(id) {
					svc := logService.NewService("docker", strings.TrimPrefix(id, "docker://"), logService.After)
					s.services.RemoveService(svc)
				}
			}
		}
	}
}

func (s *Scanner) SetActives(ids map[string]struct{}) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.actives = ids
}

func (s *Scanner) Contains(id string) bool {
	_, ok := s.actives[id]
	return ok
}
