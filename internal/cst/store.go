package cst

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"time"
)

const StoreDirName = ".cst"
const eventsFile = "events.jsonl"
const lockFile = "events.lock"

func StoreDir() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, StoreDirName)
}

func EnsureStoreDir() (string, error) {
	dir := StoreDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func EventsPath() string { return filepath.Join(StoreDir(), eventsFile) }
func LockPath() string   { return filepath.Join(StoreDir(), lockFile) }

// Lock holds an OS-level advisory lock for the entire store. Used by any
// command that mutates events or needs a coherent replay snapshot.
type Lock struct {
	f *os.File
}

func AcquireLock() (*Lock, error) {
	if _, err := EnsureStoreDir(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(LockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}

// Append appends events to the journal under an already-held lock. The caller
// must hold a Lock from AcquireLock to prevent interleaving from concurrent
// writers.
func Append(events ...*Event) error {
	if len(events) == 0 {
		return nil
	}
	if _, err := EnsureStoreDir(); err != nil {
		return err
	}
	f, err := os.OpenFile(EventsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, e := range events {
		line, err := e.MarshalLine()
		if err != nil {
			return err
		}
		if _, err := f.Write(line); err != nil {
			return err
		}
	}
	return f.Sync()
}

// Replay streams every event in append order. Empty file returns no events
// and no error.
func Replay() ([]*Event, error) {
	f, err := os.Open(EventsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []*Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		e, err := UnmarshalEvent(raw)
		if err != nil {
			return nil, fmt.Errorf("replay: %w", err)
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func NewEventID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

func NewLeaseID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func NewAttemptID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func ResolveActor(cfgDefault string) string {
	if v := os.Getenv("CST_ACTOR"); v != "" {
		return v
	}
	if cfgDefault != "" {
		return cfgDefault
	}
	u, err := user.Current()
	host, _ := os.Hostname()
	if err == nil && u != nil {
		return u.Username + "@" + host
	}
	return "unknown@" + host
}
