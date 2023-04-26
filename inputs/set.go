package inputs

import (
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/checksum"
)

type (
	Empty struct{}
	Set   map[checksum.Checksum]Empty
)

func NewSet() Set {
	return make(Set)
}

func (s Set) Add(elem checksum.Checksum) {
	s[elem] = Empty{}
}

func (s Set) Has(elem checksum.Checksum) bool {
	_, ok := s[elem]
	return ok
}

func (s Set) Load(elems map[checksum.Checksum]cfg.ConfigWithFormat) Set {
	for k, _ := range elems {
		s.Add(k)
	}
	return s
}

func (s Set) Clear() Set {
	for k := range s {
		delete(s, k)
	}
	return s
}

func (src Set) Diff(dst Set) (add, del Set) {
	record := map[checksum.Checksum]int{}
	for elem := range src {
		record[elem]++
	}
	for elem := range dst {
		record[elem]++
	}

	intersection := NewSet()
	for k, v := range record {
		if v == 2 {
			intersection.Add(k)
		}
	}

	// del := dst - interaction
	del = NewSet()
	for elem := range dst {
		if intersection.Has(elem) {
			continue
		}
		del.Add(elem)
	}
	// add := src - interaction
	add = NewSet()
	for elem := range src {
		if intersection.Has(elem) {
			continue
		}
		add.Add(elem)
	}
	return add, del
}
