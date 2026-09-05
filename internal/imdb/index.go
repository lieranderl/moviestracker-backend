package imdb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

var ErrRatingNotFound = errors.New("imdb rating not found")

type Rating struct {
	ID     string `json:"Id"`
	Rating string `json:"Rating"`
	Votes  string `json:"Votes"`
}

type Index struct {
	ratings map[string]Rating
}

func ParseRatings(reader io.Reader) (*Index, error) {
	ratings := make(map[string]Rating)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 3 || fields[0] == "tconst" {
			continue
		}
		ratings[fields[0]] = Rating{ID: fields[0], Rating: fields[1], Votes: fields[2]}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan IMDb ratings: %w", err)
	}
	return &Index{ratings: ratings}, nil
}

func (i *Index) Lookup(id string) (Rating, bool) {
	if i == nil {
		return Rating{}, false
	}
	rating, ok := i.ratings[id]
	return rating, ok
}

type Snapshot struct {
	Generation int64
	Reader     io.Reader
	Unchanged  bool
}

type Source interface {
	Open(context.Context, int64) (Snapshot, error)
}

type SourceFunc func(context.Context, int64) (Snapshot, error)

func (f SourceFunc) Open(ctx context.Context, generation int64) (Snapshot, error) {
	return f(ctx, generation)
}

type state struct {
	generation int64
	index      *Index
}

type Manager struct {
	current atomic.Pointer[state]
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Refresh(ctx context.Context, source Source) error {
	snapshot, err := source.Open(ctx, m.Generation())
	if err != nil {
		return err
	}
	if snapshot.Unchanged {
		return nil
	}
	index, err := ParseRatings(snapshot.Reader)
	if err != nil {
		return err
	}
	m.current.Store(&state{generation: snapshot.Generation, index: index})
	return nil
}

func (m *Manager) Lookup(id string) (Rating, bool) {
	current := m.current.Load()
	if current == nil {
		return Rating{}, false
	}
	return current.index.Lookup(id)
}

func (m *Manager) Generation() int64 {
	current := m.current.Load()
	if current == nil {
		return 0
	}
	return current.generation
}
