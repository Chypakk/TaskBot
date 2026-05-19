package bot

import (
	"sync"
)

type UserState string

const (
	StateIdle               UserState = "idle"
	StateCreatingName       UserState = "creating:name"
	StateCreatingDate       UserState = "creating:date"
	StateCreatingImportance UserState = "creating:importance"
	StateEditingSelect      UserState = "editing:select"
	StateEditingName        UserState = "editing:name"
	StateEditingDate        UserState = "editing:date"
	StateEditingImportance  UserState = "editing:importance"
)

type FSMState struct {
	State   UserState
	TaskID  int
	Field   string // для редактирования: "name", "date", "importance"
	Payload string // доп. данные, если понадобятся
}

type FSM struct {
	mu     sync.RWMutex
	states map[int64]FSMState
}

func NewFSM() *FSM {
	return &FSM{
		states: make(map[int64]FSMState),
	}
}

func (f *FSM) Set(userID int64, state FSMState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[userID] = state
}

func (f *FSM) Get(userID int64) (FSMState, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	s, ok := f.states[userID]
	return s, ok
}

func (f *FSM) Reset(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.states, userID)
}

// Хелперы для удобства
func (f *FSM) IsCreating(userID int64) bool {
	s, ok := f.Get(userID)
	return ok && (s.State == StateCreatingName || s.State == StateCreatingDate || s.State == StateCreatingImportance)
}

func (f *FSM) IsEditing(userID int64) bool {
	s, ok := f.Get(userID)
	return ok && (s.State == StateEditingSelect || s.State == StateEditingName || s.State == StateEditingDate || s.State == StateEditingImportance)
}