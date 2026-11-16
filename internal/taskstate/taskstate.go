// Package taskstate is the reference example for the conventions enforced
// by this repository. It implements a small task lifecycle state machine.
//
// Conventions demonstrated:
//   - Functional core: every transition is a pure function of its inputs.
//     No I/O, no clocks, no globals, no hidden dependencies.
//   - Immutable by convention: Task values are copied, never mutated in
//     place. A transition returns a new Task; the receiver is unchanged.
//   - Explicit state transitions: applying an operation in a state that
//     does not allow it returns a typed, matchable error.
//   - Errors are explicit and inspectable: errors.Is and errors.As work.
//   - The zero value is never silently valid: Task must be built with New.
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

// The known states. Anything else is invalid and rejected by transitions.
const (
	Pending   State = "pending"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

// Valid reports whether s is a known state.
func (s State) Valid() bool {
	switch s {
	case Pending, Running, Succeeded, Failed, Cancelled:
		return true
	}
	return false
}

// Terminal reports whether s is a terminal state from which no transition
// is allowed. Succeeded and Cancelled are terminal. Failed is not: a failed
// task may be moved back to Pending with Reset to make another attempt.
// Unknown states are treated as non-terminal so callers keep getting explicit
// transition errors instead of a misleading "terminal".
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
// errors.Is(err, ErrIllegalTransition) or extract details with
// errors.As(err, &TransitionError{...}).
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
// Fields are exported for inspection only; treat them as read-only. Build
// tasks with New and derive new tasks by calling the transition methods.
// The zero value has an invalid state and is rejected by every operation.
type Task struct {
	// ID identifies the task. It is carried through transitions unchanged.
	ID string
	// State is the current lifecycle state.
	State State
	// Attempts counts how many times the task has been started.
	Attempts int
}

// New returns a Pending Task with the given ID.
func New(id string) Task {
	return Task{ID: id, State: Pending}
}

// Start moves a Pending task to Running and increments Attempts.
func (t Task) Start() (Task, error) {
	if t.State == Pending {
		return Task{ID: t.ID, State: Running, Attempts: t.Attempts + 1}, nil
	}
	return Task{}, t.illegal("start")
}

// Succeed moves a Running task to Succeeded.
func (t Task) Succeed() (Task, error) {
	if t.State == Running {
		return Task{ID: t.ID, State: Succeeded, Attempts: t.Attempts}, nil
	}
	return Task{}, t.illegal("succeed")
}

// Fail moves a Running task to Failed.
func (t Task) Fail() (Task, error) {
	if t.State == Running {
		return Task{ID: t.ID, State: Failed, Attempts: t.Attempts}, nil
	}
	return Task{}, t.illegal("fail")
}

// Cancel moves a Pending or Running task to Cancelled.
func (t Task) Cancel() (Task, error) {
	if t.State == Pending || t.State == Running {
		return Task{ID: t.ID, State: Cancelled, Attempts: t.Attempts}, nil
	}
	return Task{}, t.illegal("cancel")
}

// Reset moves a Failed task back to Pending so it can be started again.
// Attempts is preserved; Start increments it.
func (t Task) Reset() (Task, error) {
	if t.State == Failed {
		return Task{ID: t.ID, State: Pending, Attempts: t.Attempts}, nil
	}
	return Task{}, t.illegal("reset")
}

// illegal builds the error for any rejected operation, including attempts
// made on a Task whose state is not one of the known constants.
func (t Task) illegal(op string) error {
	return &TransitionError{Op: op, From: t.State}
}
