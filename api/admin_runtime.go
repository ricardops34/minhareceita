package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type adminSessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
}

func newAdminSessionStore() *adminSessionStore {
	return &adminSessionStore{sessions: make(map[string]time.Time)}
}

func (s *adminSessionStore) create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(12 * time.Hour)
	s.mu.Unlock()
	return token, nil
}

func (s *adminSessionStore) valid(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.sessions[token]
	if !ok || time.Now().After(expires) {
		delete(s.sessions, token)
		return false
	}
	return true
}

func (s *adminSessionStore) delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

type adminJobState struct {
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
	Logs       []string  `json:"logs"`
}

type adminJobs struct {
	mu      sync.Mutex
	current adminJobState
	command *exec.Cmd
}

func (j *adminJobs) start(kind string, args ...string) error {
	j.mu.Lock()
	if j.current.Status == "running" {
		j.mu.Unlock()
		return errors.New("já existe um trabalho em execução")
	}
	cmd := exec.Command("/usr/bin/minha-receita", args...)
	j.current = adminJobState{Kind: kind, Status: "running", StartedAt: time.Now(), Logs: []string{fmt.Sprintf("Iniciando: minha-receita %s", strings.Join(args, " "))}}
	j.command = cmd
	w := &adminJobWriter{jobs: j}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		j.current.Status = "failed"
		j.current.FinishedAt = time.Now()
		j.current.Logs = append(j.current.Logs, err.Error())
		j.mu.Unlock()
		return err
	}
	j.mu.Unlock()
	go func() {
		err := cmd.Wait()
		j.mu.Lock()
		defer j.mu.Unlock()
		j.current.FinishedAt = time.Now()
		if err != nil {
			j.current.Status = "failed"
			j.current.Logs = append(j.current.Logs, "Erro: "+err.Error())
		} else {
			j.current.Status = "completed"
			j.current.Logs = append(j.current.Logs, "Concluído com sucesso.")
		}
		j.command = nil
	}()
	return nil
}

func (j *adminJobs) snapshot() adminJobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	state := j.current
	state.Logs = append([]string(nil), state.Logs...)
	return state
}

func (j *adminJobs) cancel() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.command == nil || j.current.Status != "running" {
		return errors.New("não existe trabalho em execução")
	}
	return j.command.Process.Signal(syscall.SIGTERM)
}

type adminJobWriter struct{ jobs *adminJobs }

func (w *adminJobWriter) Write(p []byte) (int, error) {
	w.jobs.mu.Lock()
	defer w.jobs.mu.Unlock()
	for _, line := range strings.FieldsFunc(string(p), func(r rune) bool { return r == '\n' || r == '\r' }) {
		if line = strings.TrimSpace(line); line != "" {
			w.jobs.current.Logs = append(w.jobs.current.Logs, line)
		}
	}
	if len(w.jobs.current.Logs) > 500 {
		w.jobs.current.Logs = append([]string(nil), w.jobs.current.Logs[len(w.jobs.current.Logs)-500:]...)
	}
	return len(p), nil
}

func diskStats(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	return stat.Blocks * uint64(stat.Bsize), stat.Bavail * uint64(stat.Bsize), nil
}

var _ io.Writer = (*adminJobWriter)(nil)
