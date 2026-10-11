package operations

import (
	"encoding/json"
	"path/filepath"
	"time"
)

type Source struct {
	Revision  string    `json:"revision"`
	Hash      string    `json:"hash"`
	Content   string    `json:"content"`
	AppliedAt time.Time `json:"appliedAt"`
}

func (s *Store) SaveSource(path, revision, hash, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources[path] = Source{revision, hash, content, time.Now().UTC()}
	s.persistSources()
}
func (s *Store) RemoveSource(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sources, path)
	s.persistSources()
}
func (s *Store) persistSources() {
	if s.path == "" {
		return
	}
	data, err := json.Marshal(s.sources)
	if err == nil {
		err = writeAtomic(filepath.Join(filepath.Dir(s.path), "applied-sources.json"), data)
	}
	if err != nil {
		s.sourceError = err.Error()
	} else {
		s.sourceError = ""
	}
}
func (s *Store) AppliedSource(path string) (Source, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.sources[path]
	return source, ok
}
