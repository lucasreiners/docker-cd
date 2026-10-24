// Package operations retains bounded, server-owned operation summaries.
package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Step struct {
	At      time.Time `json:"at"`
	Phase   string    `json:"phase"`
	Message string    `json:"message"`
}
type Record struct {
	runtimeVersion uint64
	Version        uint64     `json:"version"`
	ID             string     `json:"id"`
	ParentID       string     `json:"parentId,omitempty"`
	Kind           string     `json:"kind"`
	Trigger        string     `json:"trigger"`
	Stack          string     `json:"stack,omitempty"`
	Revision       string     `json:"revision,omitempty"`
	Status         string     `json:"status"`
	Message        string     `json:"message"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	Steps          []Step     `json:"steps"`
}
type contextKey struct{}
type triggerKey struct{}

func WithTrigger(ctx context.Context, trigger string) context.Context {
	return context.WithValue(ctx, triggerKey{}, trigger)
}
func Trigger(ctx context.Context) string {
	v, _ := ctx.Value(triggerKey{}).(string)
	if v == "" {
		return "automatic"
	}
	return v
}
func ID(ctx context.Context) string { id, _ := ctx.Value(contextKey{}).(string); return id }
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

type Store struct {
	mu               sync.RWMutex
	path             string
	records          map[string]Record
	lastError        string
	sources          map[string]Source
	sourceError      string
	terminalRevision uint64
}

// An empty path creates an in-memory store for isolated unit/integration fixtures.
func Open(path string) (*Store, error) {
	s := &Store{path: path, records: make(map[string]Record), sources: make(map[string]Source), terminalRevision: 1}
	if path == "" {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(data, &s.records); err != nil {
			return nil, fmt.Errorf("read operation history: %w", err)
		}
	}
	if s.records == nil {
		s.records = make(map[string]Record)
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "applied-sources.json")); err == nil {
		if err = json.Unmarshal(data, &s.sources); err != nil {
			return nil, fmt.Errorf("read applied sources: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.sources == nil {
		s.sources = make(map[string]Source)
	}
	now := time.Now().UTC()
	for id, r := range s.records {
		if r.Status == "running" {
			r.Version++
			r.Status = "interrupted"
			r.Message = "Agent restarted before this operation finished"
			r.FinishedAt = &now
			s.records[id] = r
		}
	}
	if err = s.persist(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) persist() error {
	if s.path == "" {
		return nil
	}
	data, err := json.Marshal(s.records)
	if err != nil {
		return err
	}
	return writeAtomic(s.path, data)
}
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".operations-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (s *Store) save() {
	// A completed ancestor is still live context while any descendant runs.
	protected := make(map[string]bool)
	for _, r := range s.records {
		if r.Status != "running" {
			continue
		}
		for id := r.ID; id != "" && !protected[id]; {
			protected[id] = true
			ancestor, ok := s.records[id]
			if !ok {
				break
			}
			id = ancestor.ParentID
		}
	}
	completed := make([]Record, 0)
	for _, r := range s.records {
		if r.Status != "running" && !protected[r.ID] {
			completed = append(completed, r)
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].StartedAt.After(completed[j].StartedAt) })
	for i := 1000; i < len(completed); i++ {
		delete(s.records, completed[i].ID)
	}
	if err := s.persist(); err != nil {
		s.lastError = err.Error()
	} else {
		s.lastError = ""
	}
}
func (s *Store) Start(parent, kind, trigger, stack, revision string) Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := Record{Version: 1, ID: uuid.NewString(), ParentID: parent, Kind: kind, Trigger: trigger, Stack: stack, Revision: revision, Status: "running", StartedAt: time.Now().UTC(), Steps: []Step{}}
	s.records[r.ID] = r
	s.save()
	return r
}
func (s *Store) Update(id, phase, message string, finish bool) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok {
		return r, false
	}
	if r.Status != "running" {
		return clone(r), false
	}
	now := time.Now().UTC()
	r.Message = message
	r.Version++
	r.Steps = append(r.Steps, Step{At: now, Phase: phase, Message: message})
	if len(r.Steps) > 100 {
		r.Steps = r.Steps[len(r.Steps)-100:]
	}
	if finish {
		r.Status = phase
		r.FinishedAt = &now
		if r.Kind == "deployment" || r.Kind == "removal" {
			s.terminalRevision++
			r.runtimeVersion = s.terminalRevision
		}
	}
	s.records[id] = r
	s.save()
	return clone(r), true
}

// RuntimeTerminals avoids copying steps/messages or sorting operation history on
// cache reads. Metadata is rebuilt only when a relevant operation terminates.
// Only requested discovery scopes are retained; pass since=0 when scopes change.
func (s *Store) RuntimeTerminals(since uint64, scopes map[string]struct{}) (uint64, map[string]uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if since == s.terminalRevision {
		return s.terminalRevision, nil
	}
	out := make(map[string]uint64)
	for _, r := range s.records {
		if _, known := scopes[r.Stack]; !known {
			continue
		}
		if (r.Kind == "deployment" || r.Kind == "removal") && r.Status != "running" {
			// Restored terminal history predates this process. In-process
			// generations, unlike wall time, cannot miss a clock-regressing event.
			out[r.Stack] = max(out[r.Stack], max(uint64(1), r.runtimeVersion))
		}
	}
	return s.terminalRevision, out
}
func clone(r Record) Record {
	r.Steps = append([]Step{}, r.Steps...)
	if r.FinishedAt != nil {
		finished := *r.FinishedAt
		r.FinishedAt = &finished
	}
	return r
}
func (s *Store) Get(id string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	return clone(r), ok
}
func (s *Store) List() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, clone(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}
func (s *Store) Health() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	message := s.lastError
	if s.sourceError != "" {
		message += " Applied source: " + s.sourceError
	}
	return s.path != "", message
}
