package set

type (
	Empty                          struct{}
	Set[R comparable]              map[R]Empty
	Rangeable[K comparable, V any] map[K]V
)

func New[R comparable]() Set[R] {
	return make(Set[R])
}

func (s Set[R]) Add(elem R) {
	s[elem] = Empty{}
}

func (s Set[R]) Has(elem R) bool {
	_, ok := s[elem]
	return ok
}

func NewWithLoad[R comparable, V any](elems map[R]V) Set[R] {
	s := New[R]()
	for k := range elems {
		s.Add(k)
	}
	return s
}

func (s Set[R]) Clear() Set[R] {
	for k := range s {
		delete(s, k)
	}
	return s
}

func (src Set[R]) Diff(dst Set[R]) (add, intersection, del Set[R]) {
	record := map[R]int{}
	for elem := range src {
		record[elem]++
	}
	for elem := range dst {
		record[elem]++
	}

	intersection = New[R]()
	for k, v := range record {
		if v == 2 {
			intersection.Add(k)
		}
	}

	// del := dst - interaction
	del = New[R]()
	for elem := range dst {
		if intersection.Has(elem) {
			continue
		}
		del.Add(elem)
	}
	// add := src - interaction
	add = New[R]()
	for elem := range src {
		if intersection.Has(elem) {
			continue
		}
		add.Add(elem)
	}
	return add, intersection, del
}
