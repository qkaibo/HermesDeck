package task

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type Status string

const StatusRunning Status = "running"
const StatusComplete Status = "completed"
const StatusFailed Status = "failed"
const StatusStopped Status = "stopped"

type Task struct {
	ID        string
	Command   string
	Status    Status
	Output    string
	CreatedAt time.Time
	pid       int
	cancel    context.CancelFunc
}

type Manager struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	nextID int
}

func NewManager() *Manager {
	return &Manager{tasks: make(map[string]*Task)}
}

func (m *Manager) Start(ctx context.Context, command string) (*Task, error) {
	m.mu.Lock()
	m.nextID++
	task := &Task{
		ID: fmt.Sprintf("task_%d", m.nextID), Command: command,
		Status: StatusRunning, CreatedAt: time.Now(),
	}
	m.tasks[task.ID] = task
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	task.cancel = cancel
	go func() {
		defer cancel()
		log.Printf("Task %s started: %s", task.ID, command)
		time.Sleep(100 * time.Millisecond)
		m.mu.Lock()
		task.Status = StatusComplete
		task.Output = fmt.Sprintf("[task %s completed]", task.ID)
		m.mu.Unlock()
	}()
	return task, nil
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	if task.cancel != nil {
		task.cancel()
	}
	task.Status = StatusStopped
	return nil
}

func (m *Manager) Get(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	return t, ok
}

func (m *Manager) List() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		r = append(r, t)
	}
	return r
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, task := range m.tasks {
		if task.cancel != nil {
			task.cancel()
		}
		task.Status = StatusStopped
		delete(m.tasks, id)
	}
}
