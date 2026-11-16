// Package taskstate is the reference example for the conventions enforced
// by this repository. It implements a small task lifecycle state machine.
//
// Conventions demonstrated:
//   - Functional core: every transition is a pure function of its inputs.
//     No I/O, no clocks, no globals, no hidden dependencies.
//   - Immutable by convention: Task has no exported fields. Values are
//     constructed with New and derived with the transition methods; a state
//     transition returns a new Task and never mutates the receiver.
//   - Invariants by construction: an unknown or inconsistent state cannot be
//     built through the public API, so operations never need to validate it.
//   - Explicit state transitions: applying an operation in a state that
//     does not allow it returns a typed, matchable error.
//   - Errors are explicit and inspectable: errors.Is and errors.AsType work.
//
// It is deliberately small. It is not a framework, a dependency, or a
// recommendation to model your domain as state machines. It exists so that
// agents and humans have one short, verified example to imitate.
package taskstate

import (
	"errors"
	"fmt"
)

// State is the lifecycle state of a task.
type State string

// The known states.
const (
	Pending   State = "pending"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

// Terminal reports whether s is a terminal state from which no transition
// is allowed. Succeeded and Cancelled are terminal. Failed is not: a failed
// task may be moved back to Pending with Reset to make another attempt.
// Unknown states are treated as non-terminal so callers keep getting
// explicit transition errors instead of a misleading "terminal".
func (s State) Terminal() bool {
	switch s {
	case Succeeded, Cancelled:
		return true
	case Pending, Running, Failed:
		return false
	}
	return false
}

// ErrIllegalTransition is wrapped by every error returned when an operation
// is applied in a state that does not allow it. Match it with errors.Is.
var ErrIllegalTransition = errors.New("illegal transition")

// TransitionError describes a rejected transition. Match the sentinel with
// errors.Is(err, ErrIllegalTransition), which works for any wrapped form.
// Extract the structured details with errors.AsType:
//
//	if te, ok := errors.AsType[*taskstate.TransitionError](err); ok {
//		fmt.Println(te.Op, te.From)
//	}
//
// errors.AsType replaced the older errors.As idiom; the equivalent As form
// is var te *taskstate.TransitionError + errors.As(err, &te) — note the
// target is a pointer to the pointer-holding variable, never
// errors.As(err, &TransitionError{}).
type TransitionError struct {
	// Op is the attempted operation, e.g. "start".
	Op string
	// From is the state in which the operation was attempted.
	From State
}

// Error implements error.
func (e *TransitionError) Error() string {
	return fmt.Sprintf("%v: cannot %s from state %q", ErrIllegalTransition, e.Op, e.From)
}

// Unwrap returns ErrIllegalTransition.
func (e *TransitionError) Unwrap() error { return ErrIllegalTransition }

// Task is an immutable snapshot of a tracked unit of work.
//
// The fields are unexported on purpose: the invariants (State is always one
// of the known constants; Attempts never decreases) hold by construction
// and cannot be broken by callers, callers in other packages, or copies
// taken through reflection-free means. Construct tasks with New and derive
// new tasks with the transition methods; read the values with ID, State and
// Attempts. The zero value is distinct from anything New can build and is
// rejected by every operation.
type Task struct {
	id       string
	state    State
	attempts int
}

// New returns a Pending Task with the given ID.
func New(id string) Task {
	return Task{id: id, state: Pending}
}

// ID returns the task's identifier.
func (t Task) ID() string { return t.id }

// State returns the current lifecycle state.
func (t Task) State() State { return t.state }

// Attempts returns how many times the task has been started.
func (t Task) Attempts() int { return t.attempts }

// Start moves a Pending task to Running and increments Attempts.
func (t Task) Start() (Task, error) {
	if t.state == Pending {
		return Task{id: t.id, state: Running, attempts: t.attempts + 1}, nil
	}
	return Task{}, t.illegal("start")
}

// Succeed moves a Running task to Succeeded.
func (t Task) Succeed() (Task, error) {
	if t.state == Running {
		return Task{id: t.id, state: Succeeded, attempts: t.attempts}, nil
	}
	return Task{}, t.illegal("succeed")
}

// Fail moves a Running task to Failed.
func (t Task) Fail() (Task, error) {
	if t.state == Running {
		return Task{id: t.id, state: Failed, attempts: t.attempts}, nil
	}
	return Task{}, t.illegal("fail")
}

// Cancel moves a Pending or Running task to Cancelled.
func (t Task) Cancel() (Task, error) {
	if t.state == Pending || t.state == Running {
		return Task{id: t.id, state: Cancelled, attempts: t.attempts}, nil
	}
	return Task{}, t.illegal("cancel")
}

// Reset moves a Failed task back to Pending so it can be started again.
// Attempts is preserved; Start increments it.
func (t Task) Reset() (Task, error) {
	if t.state == Failed {
		return Task{id: t.id, state: Pending, attempts: t.attempts}, nil
	}
	return Task{}, t.illegal("reset")
}

// illegal builds the error for any rejected operation, including attempts
// made on a zero-value Task whose state is not one of the known constants.
func (t Task) illegal(op string) error {
	return &TransitionError{Op: op, From: t.state}
}
