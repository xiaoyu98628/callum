package registry

import (
	"encoding/json"
	"regexp"
	"slices"
	"sync"

	"github.com/xiaoyu98628/callum/internal/tool"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Registry 持有工具实例，注册后不可替换同名工具，查找支持并发。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]tool.Tool
}

func New() *Registry { return &Registry{tools: make(map[string]tool.Tool)} }

func (r *Registry) Register(t tool.Tool) error {
	if t == nil {
		return tool.ErrInvalid
	}
	d := t.Definition()
	if !namePattern.MatchString(d.Name) || d.Description == "" || d.Source == "" || d.Version == "" || !json.Valid(d.Parameters) {
		return tool.ErrInvalid
	}
	for _, effect := range d.Effects {
		if !effect.Valid() {
			return tool.ErrInvalid
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[d.Name]; ok {
		return tool.ErrInvalid
	}
	r.tools[d.Name] = t
	return nil
}

func (r *Registry) Get(name string) (tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tools[name]
	if t == nil {
		return nil, tool.ErrUnavailable
	}
	return t, nil
}

func (r *Registry) Definitions() []tool.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]tool.Definition, 0, len(r.tools))
	for _, t := range r.tools {
		d := t.Definition()
		d.Parameters = slices.Clone(d.Parameters)
		d.Effects = slices.Clone(d.Effects)
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b tool.Definition) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}
