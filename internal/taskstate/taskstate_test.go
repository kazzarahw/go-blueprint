package taskstate_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kazzarahw/go-blueprint/internal/taskstate"
)

// allowed encodes the transition table once, as data. The tests below
// check every (operation, from-state) pair against it, so the spec is
// stated exactly once and can never silently drift from the tests.
var allowed = map[string]map[taskstate.State]taskstate.State{
	"start":   {taskstate.Pending: taskstate.Running},
	"succeed": {taskstate.Running: taskstate.Succeeded},
	"fail":    {taskstate.Running: taskstate.Failed},
	"cancel":  {taskstate.Pending: taskstate.Cancelled, taskstate.Running: taskstate.Cancelled},
	"reset":   {taskstate.Failed: taskstate.Pending},
}

type operation struct {
	name string
	call func(taskstate.Task) (taskstate.Task, error)
}

var operations = []operation{
	{"start", taskstate.Task.Start},
	{"succeed", taskstate.Task.Succeed},
	{"fail", taskstate.Task.Fail},
	{"cancel", taskstate.Task.Cancel},
	{"reset", taskstate.Task.Reset},
}

var fromStates = []taskstate.State{
	taskstate.Pending,
	taskstate.Running,
	taskstate.Succeeded,
	taskstate.Failed,
	taskstate.Cancelled,
	taskstate.State("bogus"), // invalid states must be rejected, never panic
}

func TestTransitions_CompleteMatrix(t *testing.T) {
	for _, op := range operations {
		for _, from := range fromStates {
			t.Run(fmt.Sprintf("%s_from_%s", op.name, from), func(t *testing.T) {
				checkTransition(t, op, from)
			})
		}
	}
}

func checkTransition(t *testing.T, op operation, from taskstate.State) {
	t.Helper()
	task := taskstate.Task{ID: "task-1", State: from, Attempts: 3}

	next, err := op.call(task)
	if want, ok := allowed[op.name][from]; ok {
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if next.State != want {
			t.Errorf("state = %q, want %q", next.State, want)
		}
		if next.ID != task.ID {
			t.Errorf("ID = %q, want %q", next.ID, task.ID)
		}
		return
	}
	if !errors.Is(err, taskstate.ErrIllegalTransition) {
		t.Errorf("error = %v, want errors.Is(err, ErrIllegalTransition)", err)
	}
	if next != (taskstate.Task{}) {
		t.Errorf("task = %+v, want zero Task on error", next)
	}
}

func TestTransitions_AreImmutable(t *testing.T) {
	original := taskstate.New("task-1")
	started, err := original.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if original.State != taskstate.Pending || original.Attempts != 0 {
		t.Errorf("original mutated: %+v", original)
	}
	if started.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", started.Attempts)
	}
}

func TestStart_PreservesID_AndCountsAttemptsAcrossRetries(t *testing.T) {
	task := taskstate.New("task-1")
	for want := 1; want <= 2; want++ {
		var err error
		task, err = task.Start()
		if err != nil {
			t.Fatalf("Start (attempt %d): %v", want, err)
		}
		if task.Attempts != want {
			t.Fatalf("Attempts = %d, want %d", task.Attempts, want)
		}
		if want < 2 {
			task = failThenReset(t, task)
		}
	}
	if task.ID != "task-1" {
		t.Errorf("ID = %q, want %q", task.ID, "task-1")
	}
}

func failThenReset(t *testing.T, task taskstate.Task) taskstate.Task {
	t.Helper()
	task, err := task.Fail()
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}
	task, err = task.Reset()
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	return task
}

func TestTerminalStates_RejectEveryOperation(t *testing.T) {
	for _, terminal := range []taskstate.State{taskstate.Succeeded, taskstate.Cancelled} {
		for _, op := range operations {
			task := taskstate.Task{ID: "task-1", State: terminal, Attempts: 1}
			if next, err := op.call(task); err == nil {
				t.Errorf("%s from terminal state %q succeeded: %+v", op.name, terminal, next)
			}
		}
	}
}

func TestTransitionError_IsMatchable(t *testing.T) {
	_, err := taskstate.New("task-1").Succeed()
	var te *taskstate.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As failed for %v", err)
	}
	if te.Op != "succeed" || te.From != taskstate.Pending {
		t.Errorf("TransitionError = %+v, want Op=succeed From=pending", te)
	}
	if !errors.Is(err, taskstate.ErrIllegalTransition) {
		t.Errorf("errors.Is(err, ErrIllegalTransition) = false for %v", err)
	}
}

func Example() {
	task := taskstate.New("build")
	task, err := task.Start()
	if err != nil {
		panic(err)
	}
	task, err = task.Succeed()
	if err != nil {
		panic(err)
	}
	fmt.Println(task.State)
	// Output: succeeded
}

// FuzzTransitionSequence applies a byte script of operations to a task and
// asserts the invariants the type is supposed to guarantee: no panics,
// errors are always matchable as ErrIllegalTransition, terminal states
// absorb all further operations, state stays valid, and attempts never go
// backwards. The script alphabet is deliberately untrusted input.
func FuzzTransitionSequence(f *testing.F) {
	f.Add("aa")
	f.Add("aabbccddeee")
	f.Add("")
	f.Add("\x00\xff\n")
	f.Fuzz(func(t *testing.T, script string) {
		task := taskstate.New("fuzz")
		terminal := false
		for i := range len(script) {
			next, err, ok := applyOp(task, script[i])
			if !ok {
				continue
			}
			checkInvariants(t, script[i], terminal, task, next, err)
			if err == nil {
				task = next
				terminal = task.State.Terminal()
			}
		}
	})
}

func applyOp(task taskstate.Task, b byte) (taskstate.Task, error, bool) {
	var fn func(taskstate.Task) (taskstate.Task, error)
	switch b {
	case 'a':
		fn = taskstate.Task.Start
	case 'b':
		fn = taskstate.Task.Succeed
	case 'c':
		fn = taskstate.Task.Fail
	case 'd':
		fn = taskstate.Task.Cancel
	case 'e':
		fn = taskstate.Task.Reset
	default:
		return taskstate.Task{}, nil, false
	}
	next, err := fn(task)
	return next, err, true
}

func checkInvariants(t *testing.T, op byte, terminal bool, prev, next taskstate.Task, err error) {
	t.Helper()
	if err != nil {
		if !errors.Is(err, taskstate.ErrIllegalTransition) {
			t.Fatalf("op %q from %q: error %v is not ErrIllegalTransition", op, prev.State, err)
		}
		if next != (taskstate.Task{}) {
			t.Fatalf("op %q from %q: non-zero task %+v on error", op, prev.State, next)
		}
		return
	}
	if terminal {
		t.Fatalf("op %q: transition out of terminal state %q succeeded", op, prev.State)
	}
	if !next.State.Valid() {
		t.Fatalf("op %q: state %q is not valid", op, next.State)
	}
	if next.Attempts < prev.Attempts {
		t.Fatalf("op %q: attempts regressed %d -> %d", op, prev.Attempts, next.Attempts)
	}
}

func TestState_UnknownAndZeroValueAreRejected(t *testing.T) {
	for _, s := range []taskstate.State{"", "bogus", "PENDING"} {
		if s.Valid() {
			t.Errorf("State(%q).Valid() = true, want false", s)
		}
		if s.Terminal() {
			t.Errorf("State(%q).Terminal() = true, want false", s)
		}
		// They must also be rejected by the transition engine.
		if _, err := (taskstate.Task{ID: "x", State: s}).Start(); !errors.Is(err, taskstate.ErrIllegalTransition) {
			t.Errorf("Start from State(%q): err = %v, want ErrIllegalTransition", s, err)
		}
	}
}

func TestTransitionError_MessageNamesOperationAndState(t *testing.T) {
	_, err := taskstate.Task{ID: "x", State: taskstate.Succeeded}.Start()
	var te *taskstate.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As failed for %v", err)
	}
	msg := te.Error()
	if !strings.Contains(msg, "start") || !strings.Contains(msg, "succeeded") {
		t.Errorf("Error() = %q, want it to mention operation and state", msg)
	}
}
