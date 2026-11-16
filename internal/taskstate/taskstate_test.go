package taskstate_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kazzarahw/go-blueprint/internal/taskstate"
)

// Test fixtures are built fresh by helper functions rather than stored in
// package-level variables. Package-level mutable state — even in tests —
// contradicts the convention enforced across this repository; see TESTING.md.
// The transition table below *is* the spec: every operation × from-state
// pair is checked against it, so it is stated exactly once per test run.

// allowedTransitions encodes the transition table as data.
func allowedTransitions() map[string]map[taskstate.State]taskstate.State {
	return map[string]map[taskstate.State]taskstate.State{
		"start":   {taskstate.Pending: taskstate.Running},
		"succeed": {taskstate.Running: taskstate.Succeeded},
		"fail":    {taskstate.Running: taskstate.Failed},
		"cancel":  {taskstate.Pending: taskstate.Cancelled, taskstate.Running: taskstate.Cancelled},
		"reset":   {taskstate.Failed: taskstate.Pending},
	}
}

type operation struct {
	name string
	call func(taskstate.Task) (taskstate.Task, error)
}

func operations() []operation {
	return []operation{
		{"start", taskstate.Task.Start},
		{"succeed", taskstate.Task.Succeed},
		{"fail", taskstate.Task.Fail},
		{"cancel", taskstate.Task.Cancel},
		{"reset", taskstate.Task.Reset},
	}
}

// knownStates lists the representable states. Invalid states are
// unrepresentable by construction (Task has no exported fields); only raw
// State values can be out-of-vocabulary, which Terminal must tolerate.
func knownStates() []taskstate.State {
	return []taskstate.State{
		taskstate.Pending,
		taskstate.Running,
		taskstate.Succeeded,
		taskstate.Failed,
		taskstate.Cancelled,
	}
}

func mustTask(t *testing.T, task taskstate.Task, err error) taskstate.Task {
	t.Helper()
	if err != nil {
		t.Fatalf("setup transition failed: %v", err)
	}
	return task
}

// taskInState builds a task in the given state via real transitions — the
// only construction path the public API offers.
func taskInState(t *testing.T, state taskstate.State) taskstate.Task {
	t.Helper()
	must := func(task taskstate.Task, err error) taskstate.Task {
		return mustTask(t, task, err)
	}
	fresh := func() taskstate.Task { return taskstate.New("task-1") }
	switch state {
	case taskstate.Pending:
		return fresh()
	case taskstate.Running:
		return must(fresh().Start())
	case taskstate.Succeeded:
		return must(must(fresh().Start()).Succeed())
	case taskstate.Failed:
		return must(must(fresh().Start()).Fail())
	case taskstate.Cancelled:
		return must(fresh().Cancel())
	default:
		t.Fatalf("no deterministic transition path to state %q", state)
		return taskstate.Task{}
	}
}

func TestTransitions_CompleteMatrix(t *testing.T) {
	allowed := allowedTransitions()
	for _, op := range operations() {
		for _, from := range knownStates() {
			t.Run(fmt.Sprintf("%s_from_%s", op.name, from), func(t *testing.T) {
				checkTransition(t, op, from, allowed)
			})
		}
	}
}

func checkTransition(t *testing.T, op operation, from taskstate.State, allowed map[string]map[taskstate.State]taskstate.State) {
	t.Helper()
	task := taskInState(t, from)

	next, err := op.call(task)
	if want, ok := allowed[op.name][from]; ok {
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if next.State() != want {
			t.Errorf("state = %q, want %q", next.State(), want)
		}
		if next.ID() != task.ID() {
			t.Errorf("ID = %q, want %q", next.ID(), task.ID())
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

func TestZeroValue_TaskRejectedEveryOperation(t *testing.T) {
	var zero taskstate.Task
	for _, op := range operations() {
		if next, err := op.call(zero); err == nil {
			t.Errorf("%s on zero Task succeeded: %+v", op.name, next)
		} else if !errors.Is(err, taskstate.ErrIllegalTransition) {
			t.Errorf("%s on zero Task: err = %v, want errors.Is(err, ErrIllegalTransition)", op.name, err)
		}
	}
}

func TestTransitions_AreImmutable(t *testing.T) {
	original := taskstate.New("task-1")
	started, err := original.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if original.State() != taskstate.Pending || original.Attempts() != 0 {
		t.Errorf("original mutated: %+v", original)
	}
	if started.Attempts() != 1 {
		t.Errorf("Attempts = %d, want 1", started.Attempts())
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
		if task.Attempts() != want {
			t.Fatalf("Attempts = %d, want %d", task.Attempts(), want)
		}
		if want < 2 {
			task = failThenReset(t, task)
		}
	}
	if task.ID() != "task-1" {
		t.Errorf("ID = %q, want %q", task.ID(), "task-1")
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
		for _, op := range operations() {
			task := taskInState(t, terminal)
			if next, err := op.call(task); err == nil {
				t.Errorf("%s from terminal state %q succeeded: %+v", op.name, terminal, next)
			}
		}
	}
}

func TestState_UnknownValuesAreNonTerminal(t *testing.T) {
	for _, s := range []taskstate.State{"", "bogus", "PENDING"} {
		if s.Terminal() {
			t.Errorf("State(%q).Terminal() = true, want false", s)
		}
	}
}

func TestTransitionError_IsMatchable(t *testing.T) {
	_, err := taskstate.New("task-1").Succeed()

	te, ok := errors.AsType[*taskstate.TransitionError](err)
	if !ok {
		t.Fatalf("errors.AsType failed for %v", err)
	}
	if te.Op != "succeed" || te.From != taskstate.Pending {
		t.Errorf("TransitionError = %+v, want Op=succeed From=pending", te)
	}
	if !errors.Is(err, taskstate.ErrIllegalTransition) {
		t.Errorf("errors.Is(err, ErrIllegalTransition) = false for %v", err)
	}

	// The classic errors.As idiom remains valid with a pointer-to-pointer
	// target.
	var te2 *taskstate.TransitionError
	if !errors.As(err, &te2) {
		t.Fatalf("errors.As failed for %v", err)
	}
	if *te2 != *te {
		t.Errorf("errors.As and errors.AsType disagree: %+v vs %+v", te2, te)
	}
}

func TestTransitionError_MessageNamesOperationAndState(t *testing.T) {
	_, err := taskstate.Task{}.Start()
	var te *taskstate.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As failed for %v", err)
	}
	msg := te.Error()
	if !strings.Contains(msg, "start") || !strings.Contains(msg, "\"\"") {
		t.Errorf("Error() = %q, want it to mention operation and the zero state", msg)
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
	fmt.Println(task.State())
	// Output: succeeded
}

// FuzzTransitionSequence applies a byte script of operations to a task and
// asserts the invariants the type is supposed to guarantee: no panics,
// errors are always matchable as ErrIllegalTransition, terminal states
// absorb all further operations, state stays one of the known constants, and
// attempts never go backwards. The script alphabet is deliberately
// untrusted input.
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
				terminal = task.State().Terminal()
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
			t.Fatalf("op %q from %q: error %v is not ErrIllegalTransition", op, prev.State(), err)
		}
		if next != (taskstate.Task{}) {
			t.Fatalf("op %q from %q: non-zero task %+v on error", op, prev.State(), next)
		}
		return
	}
	if terminal {
		t.Fatalf("op %q: transition out of terminal state %q succeeded", op, prev.State())
	}
	if !knownState(next.State()) {
		t.Fatalf("op %q: state %q is not one of the known constants", op, next.State())
	}
	if next.Attempts() < prev.Attempts() {
		t.Fatalf("op %q: attempts regressed %d -> %d", op, prev.Attempts(), next.Attempts())
	}
}

func knownState(s taskstate.State) bool {
	for _, known := range knownStates() {
		if s == known {
			return true
		}
	}
	return false
}
