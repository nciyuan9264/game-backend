package roomcore

import "sync"

// Registry 是泛型化的进程内房间注册表。
type Registry[R any] struct {
	mu sync.Mutex
	m  map[string]R
}

// NewRegistry 构造一个空的注册表。
func NewRegistry[R any]() *Registry[R] {
	return &Registry[R]{m: map[string]R{}}
}

// Get 返回指定房间，若不存在 ok=false。
func (r *Registry[R]) Get(id string) (R, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[id]
	return v, ok
}

// Set 写入或覆盖房间。
func (r *Registry[R]) Set(id string, v R) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[id] = v
}

// Delete 删除房间。
func (r *Registry[R]) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}

// Snapshot 返回当前所有房间的浅拷贝。
func (r *Registry[R]) Snapshot() map[string]R {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]R, len(r.m))
	for k, v := range r.m {
		cp[k] = v
	}
	return cp
}

// Len 返回当前房间数量。
func (r *Registry[R]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}

// Clear 清空所有房间。
func (r *Registry[R]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m = map[string]R{}
}
