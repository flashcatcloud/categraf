//go:build !no_logs

package kubernetes

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	logService "flashcat.cloud/categraf/logs/service"
	"flashcat.cloud/categraf/logs/util/containers"
	"flashcat.cloud/categraf/logs/util/kubernetes/kubelet"
	"flashcat.cloud/categraf/pkg/checksum"
	"flashcat.cloud/categraf/pkg/set"
)

type (
	Scanner struct {
		kubelet  kubelet.KubeUtilInterface
		services *logService.Services
		actives  map[string]checksum.Checksum
		mux      sync.Mutex
	}
)

func NewScanner(services *logService.Services) *Scanner {
	return &Scanner{
		services: services,
		actives:  make(map[string]checksum.Checksum),
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
				continue
			}
			fetched := make(map[string]checksum.Checksum)
			for _, pod := range pods {
				for _, container := range pod.Status.GetAllContainers() {
					fetched[container.ID] = checksum.New(pod.Metadata)
				}
			}
			new := set.NewWithLoad[string, checksum.Checksum](fetched)
			old := set.NewWithLoad[string, checksum.Checksum](s.GetActives())
			add, checkTwice, del := new.Diff(old)
			for id := range del {
				rtype, rid := parseEntity(id)
				svc := logService.NewService(rtype, rid, logService.After)
				s.services.RemoveService(svc)
				s.DelActives(id)
			}

			for id := range checkTwice {
				sum := fetched[id]
				if !s.Contains(id, sum) {
					rtype, rid := parseEntity(id)
					svc := logService.NewService(rtype, rid, logService.After)
					s.services.RemoveService(svc)
					svc = logService.NewService(rtype, rid, logService.After)
					s.services.AddService(svc)
					s.AddActives(id, sum)
				}
			}

			for id := range add {
				rtype, rid := parseEntity(id)
				svc := logService.NewService(rtype, rid, logService.After)
				s.services.AddService(svc)
				s.AddActives(id, fetched[id])
			}

		}
	}
}

func parseEntity(containerID string) (string, string) {
	components := strings.Split(containerID, containers.EntitySeparator)
	if len(components) != 2 {
		return "docker", strings.TrimPrefix(containerID, "docker"+containers.EntitySeparator)
	}
	return components[0], components[1]
}

func (s *Scanner) SetActives(ids map[string]checksum.Checksum) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.actives = ids
}

func (s *Scanner) GetActives() map[string]checksum.Checksum {
	ret := make(map[string]checksum.Checksum)
	s.mux.Lock()
	defer s.mux.Unlock()
	for k, v := range s.actives {
		ret[k] = v
	}
	return ret
}

func (s *Scanner) AddActives(id string, sum checksum.Checksum) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.actives[id] = sum
}
func (s *Scanner) DelActives(id string) {
	s.mux.Lock()
	defer s.mux.Unlock()
	delete(s.actives, id)
}

func (s *Scanner) Contains(id string, sum checksum.Checksum) bool {
	val, ok := s.actives[id]
	return ok && val == sum
}
