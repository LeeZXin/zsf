package hashset

type HashSet[T comparable] map[T]struct{}

func NewHashSet[T comparable](l []T) HashSet[T] {
	if l == nil {
		return make(HashSet[T])
	}
	ret := make(HashSet[T], len(l))
	for i := range l {
		ret.Add(l[i])
	}
	return ret
}

func (s HashSet[T]) Add(key T) {
	s[key] = struct{}{}
}

func (s HashSet[T]) Delete(key T) {
	delete(s, key)
}

func (s HashSet[T]) Contains(key T) bool {
	_, ok := s[key]
	return ok
}

func (s HashSet[T]) Copy() HashSet[T] {
	ret := make(HashSet[T], len(s))
	for k, v := range s {
		ret[k] = v
	}
	return ret
}

func (s HashSet[T]) ToSlice() []T {
	ret := make([]T, 0, len(s))
	for k := range s {
		ret = append(ret, k)
	}
	return ret
}

func (s HashSet[T]) Intersect(h HashSet[T]) HashSet[T] {
	ret := make(HashSet[T], 8)
	if h != nil {
		for k := range s {
			_, ok := h[k]
			if ok {
				ret[k] = struct{}{}
			}
		}
	}
	return ret
}

func (s HashSet[T]) Remove(n HashSet[T]) HashSet[T] {
	c := s.Copy()
	for k := range c {
		if n.Contains(k) {
			delete(c, k)
		}
	}
	return c
}
