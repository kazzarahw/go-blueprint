# TESTING.md — Testing philosophy and mechanics

Testing in this repository is not a phase after implementation; it is the
mechanism by which a change is *understood*. AI agents generate code faster
than humans can review it — deterministic, adversarial tests are the reason
that is safe.

## Rules

1. **Tests ship with the change.** A behavior without a test is a behavior
   nobody verified. Write them together.
2. **Deterministic or it does not exist.** No wall clock, no network, no
   sleeps, no `math/rand` without injecting the source, no filesystem
   outside `t.TempDir()`, no dependence on test execution order.
3. **Table-driven by default.** The table is the spec. Enumerate the
   (input → expected) space explicitly.
4. **Assert invariants and behavior, not trivia.** Never pin error message
   strings; pin `errors.Is`/`errors.AsType` matches instead. Never assert on
   implementation details you expect to refactor.
5. **The race detector is always on** (`make test` passes `-race -count=1`).
6. **Fuzz what is adversarial**: parsers, decoders, state machines, anything
   whose inputs are untrusted. Crashers become committed corpus seeds.
7. **Tests are code**: they must pass the same lint/format/static gate as
   everything else.

## Layout and mechanics

- Test files: `foo_test.go` next to `foo.go`, package `foo_test` (black-box)
  by default; use internal test package only when testing unexported
  behavior is genuinely necessary.
- Command surface:
  | Command | Purpose |
  |---|---|
  | `make test` | `go test -race -count=1 ./...` |
  | `make cover` | coverage report (no threshold gate; see below) |
  | `make fuzz FUZZTIME=30s` | fuzz every `Fuzz*` target for a fixed time |
  | `go test -run TestName ./internal/pkg/` | single test |
  | `make smoke` | end-to-end: copy template → rename module → `make verify` there |
  | `go test -v ./...` | verbose |

## A worked example

`internal/taskstate` is the reference. Its test file shows the house style:

- **Invariants by construction**: `Task` has no exported fields, so an
  unknown `State` cannot be embedded in a value from outside the package.
  The matrix test therefore covers the five known states; the zero value
  (the only invalid constructible task) is rejected by every operation and
  is tested explicitly.
- **The transition table is data** (`allowedTransitions()`), and
  `TestTransitions_CompleteMatrix` checks *every* (operation × from-state)
  pair against it — the spec appears exactly once per test run.
- **Fixtures are built per test**, not stored in package-level variables:
  `allowedTransitions()`, `operations()` and `knownStates()` are
  constructor functions; `taskInState` builds states via real transitions.
  Package-level mutable state is forbidden even in tests — see
  "Package-level state in tests" below.
- **Immutability is asserted**: after a transition, the original value is
  unchanged and the derived value carries the change.
- **Retry semantics are asserted behaviorally**, not by poking internals:
  start → fail → reset → start increments `Attempts` to 2.
- **Errors are matchable**: `errors.AsType[*taskstate.TransitionError]`
  extracts the structured details (the classic `errors.As` with a
  pointer-to-pointer target is also shown); `errors.Is` reaches the
  sentinel.
- **`Example()`** doubles as executable documentation with deterministic
  output.

Copy this pattern when building your own domain types.

## Package-level state in tests

The ban on package-level mutable state (AGENTS.md §1) applies to test files
too — maps, slices and buffers stored as package-level `var` are mutable
state, whatever the test file's excuse. Rules:

1. Fixtures are built inside the test (or in `TestXxx`-scoped helpers
   taking `t *testing.T`), never shared. See the constructor functions in
   `internal/taskstate/taskstate_test.go`.
2. Sentinel errors and other genuinely immutable values may live at package
   level (they already do in non-test code). Anything whose fields or
   contents a test could mutate must not.
3. Table-driven cases are built per test function; mutation of one case
   must never be able to leak into another test.

## Writing a table-driven test

```go
func TestParseThing(t *testing.T) {
    tests := []struct {
        name    string
        give    string
        want    Thing
        wantErr error // sentinel to match with errors.Is, nil for success
    }{
        {name: "empty", give: "", wantErr: ErrEmpty},
        {name: "simple", give: "a=b", want: Thing{A: "a", B: "b"}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseThing(tt.give)
            if tt.wantErr != nil {
                if !errors.Is(err, tt.wantErr) {
                    t.Fatalf("err = %v, want errors.Is match for %v", err, tt.wantErr)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if got != tt.want {
                t.Errorf("got %+v, want %+v", got, tt.want)
            }
        })
    }
}
```

Complex per-case setup belongs in a helper function taking `t *testing.T`
(call `t.Helper()`), not in the loop body — `gocognit` enforces small
functions everywhere, tests included.

## Determinism patterns

- **Time**: inject `func() time.Time` or a small `Clock` interface. Never
  `time.Now()` in logic you want to test.
- **Randomness**: inject `*math/rand.Rand` seeded in the test, or pass an
  interface.
- **IDs**: inject an ID generator or accept a fixed set in tests.
- **Sleeps**: a sleep in a test is a bug report about your design. In
  goroutine tests, synchronize on channels/`sync.WaitGroup`; use
  `t.Deadline()`-aware selects.
- **Filesystem**: `t.TempDir()`. **Network**: `httptest` servers, or fakes.

## Fuzzing

Go's native fuzzing (`testing.F`) is wired into `make fuzz` and runs in CI
as a 30s smoke pass per target.

```go
func FuzzTransitionSequence(f *testing.F) {
    f.Add("aabbcc")      // seed corpus — commit these
    f.Fuzz(func(t *testing.T, script string) {
        // drive the system with the script; assert invariants;
        // t.Fatalf on any violation (never panic for expected failures)
    })
}
```

Guidelines:

- Fuzz functions assert **invariants**, not outputs: no panics, errors
  match the sentinel, terminal states absorb further operations, monotonic
  counters never regress. See `FuzzTransitionSequence` for the pattern.
- Every fixed crasher becomes a seed in `f.Add` (or a file in
  `testdata/fuzz/<Name>/`) so regressions are caught forever.
- Fuzz for ≥30s locally before finishing a parser/state-machine change;
  CI does a smoke pass, not a campaign.

## Coverage

`make cover` prints per-function coverage. There is intentionally **no
numeric gate**: thresholds reward gaming (tests that execute code without
asserting) and punish hard-to-test defensive branches that exist for safety.
Use coverage to *find untested behavior*, not to prove quality. If your
project wants a gate, add it with a rationale in an ADR — never silently.

## What not to test

- Standard library behavior (trust it; pin it only via your own contract).
- Exact error strings (match sentinels/types instead).
- Logging output (assert state, not log bytes) — unless logs are the contract.
- Generated code and one-line getters/setters.
- Migration/compat shims in other people's modules.

## Reviewing a test change (agents reviewing agents)

A test change is suspicious when it:

- deletes or weakens assertions without changing behavior,
- replaces a sentinel/type match with a string match,
- introduces nondeterminism (time, rand, sleeps, network),
- adds a `//nolint` on a test,
- covers code by calling it and never checking the result.

A good test change fails when the implementation is intentionally broken.
When reviewing, break the code on purpose and watch the test fail — that is
the cheapest mutation testing available.

## Definition of done (testing part)

- New behavior: table-driven test enumerates the spec.
- New parser / state machine / adversarial input surface: a fuzz target with seeds.
- Bug fix: a regression test that fails on the old code.
- `make test` and `make fuzz FUZZTIME=30s` are green.
