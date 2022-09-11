# Go Style Guide

- [Go Style Guide](#go-style-guide)
  - [Consistent Spelling and Naming](#consistent-spelling-and-naming)
  - [Code Formatting](#code-formatting)
  - [Unused Names](#unused-names)
  - [Naked returns and Named Parameters](#naked-returns-and-named-parameters)
  - [Testing](#testing)
    - [Parallel Test Execution](#parallel-test-execution)
    - [Naming Tests](#naming-tests)
  - [Group Declarations by Meaning](#group-declarations-by-meaning)
  - [Make Zero-value Useful](#make-zero-value-useful)
  - [Beware of Copying Mutexes in Go](#beware-of-copying-mutexes-in-go)
  - [Copy Slices and Maps at Boundaries](#copy-slices-and-maps-at-boundaries)
    - [Receiving Slices and Maps](#receiving-slices-and-maps)
    - [Returning Slices and Maps](#returning-slices-and-maps)
    - [Filtering in place](#filtering-in-place)
  - [Pointers to Interfaces](#pointers-to-interfaces)
    - [Verify Interface Compliance](#verify-interface-compliance)
  - [Receivers and Interfaces](#receivers-and-interfaces)
  - [Defer to Clean Up](#defer-to-clean-up)
  - [Channel Size is One or None](#channel-size-is-one-or-none)
  - [Start Enums at One](#start-enums-at-one)
  - [Use `"time"` to handle time](#use-time-to-handle-time)
    - [Use `time.Time` for instants of time](#use-timetime-for-instants-of-time)
    - [Use `time.Duration` for periods of time](#use-timeduration-for-periods-of-time)
    - [Use `time.Time` and `time.Duration` with external systems](#use-timetime-and-timeduration-with-external-systems)
  - [Error Types](#error-types)
    - [Error Wrapping](#error-wrapping)
    - [Handle Type Assertion Failures](#handle-type-assertion-failures)
    - [Panic with care](#panic-with-care)
  - [Avoid Mutable Globals](#avoid-mutable-globals)
  - [Avoid Embedding Types in Public Structs](#avoid-embedding-types-in-public-structs)
  - [Avoid Using Built-In Names](#avoid-using-built-in-names)
  - [Avoid `init()`](#avoid-init)
  - [Exit in Main](#exit-in-main)
    - [Exit Once](#exit-once)
  - [Performance](#performance)
    - [Prefer strconv over fmt](#prefer-strconv-over-fmt)
    - [Avoid string-to-byte conversion](#avoid-string-to-byte-conversion)
    - [Prefer Specifying Container Capacity](#prefer-specifying-container-capacity)
    - [Specifying Map Capacity Hints](#specifying-map-capacity-hints)
    - [Specifying Slice Capacity](#specifying-slice-capacity)

## Consistent Spelling and Naming

Naming is difficult, but prefer the consistency of naming throughout the code base.
If a naming pattern is already established in the codebase, follow it. If you are unsure, look in the Golang standard library for inspiration.
It's similar to the `gofmt` tool, the formatting isn't to everyone's liking, but it is consistent.

Prefer american spellings over British spellings, avoid Latin abbreviations.

*Note:* `misspell` linter should take care of these, but it's good to keep in mind.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// marshalling
// unmarshalling
// cancelling
// cancelled
// cancelation
```

</td><td>

```go
// marshaling
// unmarshaling
// canceling
// canceled
// cancellation
```

</td></tr>
</tbody></table>

## Code Formatting

- Keep line length, argument count and function size reasonable.
- Run `make lint-style` command to lint the codebase using a set of style linters.
- Run `make format FOLDER=pkg/mypackage` to format the code using `gci` and `gfumpt` formatters.

## Unused Names

Avoid unused method receiver names.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
func (f foo) method() {
	// no references to f
}

func method(_ string) {
	...
}
```

</td><td>

```go
func (foo) method() {
	...
}

func method(string) {
	...
}
```

</td></tr>
</tbody></table>

Note: there might be a linter to take care of this

## Naked returns and Named Parameters

Don't name result parameters just to avoid declaring a var inside the function; that trades off a minor implementation brevity at the cost of unnecessary API verbosity.

Naked returns are okay if the function is a handful of lines. Once it's a medium sized function, be explicit with your return values.

```go
func collect(birds ...byte) (ducks []byte) {
  for _, bird := range birds {
    if isDuck(bird) {
      ducks = append(ducks, bird)
    }
  }

  return
}
```

*Corollary:* it's not worth it to name result parameters just because it enables you to use naked returns. Clarity of docs is always more important than saving a line or two in your function.

Finally, in some cases you need to name a result parameter in order to change it in a deferred closure. That is always OK.

```go
func getID() (id int, err error) {
   defer func() {
      if err != nil {
         err = fmt.Errorf("some extra info: %v", err)
      }
   }

   id, err = /* call to db */

   return
}
```
## Testing

Use the Golang [testing package](https://pkg.go.dev/testing) from the standard library for writing tests.

### Parallel Test Execution

Run tests in parallel where possible but don't forget about variable scope gotchas.

```go
for tc := range tt {
  tc := tc // must not forget this
  t.Run(tc.name, func(t *testing.T) {
    t.Parallel()
    //execute
    // ...
    // assert
    if got != tt.want {
      // useful for human comparable values
      t.Errorf("wrong output\ngot:  %q\nwant: %q", got, want)
      // alternatively
      t.Errorf("Foo(%q) = %d; want %d", tt.in, got, tt.want)
    }
  })
}
```

### Naming Tests

Name tests with a compact name that reflects their scenario. Don't try to specify the scenario in the test name, that is not what it's for. Use the accompanying godoc to describe the test scenario.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>
```go
func TestSomethingBySettingVarToFive(t *testing.T) {
  ...
}
```
</td><td>
```go
// TestSomething tests that something works correctly by doing this and that.
func TestSomething(t *testing.T) {
  ...
}
```
</td>
</td></tr>
</tbody></table>

If needed, use an underscore to disambiguate tests that are hard to name:
```go
func TestScenario_EdgeCase(t *testing.T) {
  ...
}

func TestScenario_CornerCase(t *testing.T) {
  ...
}
```

Ideally, try to use nested tests that would cause the test runner to automatically assemble the different test cases in separate entries:
```go
func TestSomething(t *testing.T) {
  ...
  t.Run("edge case", func(t *testing.T) { ... })
}

Lastly, please, goodness, don't use the word "fail" when naming tests. Since the go test runner uses the same keyword to denote failed tests, this just prolongs the search for relevant information when inspecting build artifacts.


## Group Declarations by Meaning

Where possible, group declarations by their purpose.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
const (
	limitDays   = 90
	warningDays = 0.9 * limitDays
	sleepFor    = 30 * time.Minute
)


var (
  _                 Interface = (*Accounting)(nil)
  balancesPrefix    = "accounting_balance_"
  someOtherPrefix   = "some_other_balance_"
  ErrLimitExceeded  = errors.New("limit exeeded")
)


```

</td><td>

```go
const (
  limitDays   = 90
  warningDays = 0.9 * limitDays
)

const sleepFor = 30 * time.Minute

var _ Interface = (*Accounting)(nil)

var ( // or const
  balancesPrefix  = "accounting_balance_"
  someOtherPrefix = "some_other_balance_"
)

var ErrLimitExceeded = errors.New("limit exeeded)
```

</td></tr>
</tbody></table>


## Make Zero-value Useful

- The zero-value of `sync.Mutex` and `sync.RWMutex` is valid, so you almost never need a pointer to a mutex.

  <table>
  <thead><tr><th>Bad</th><th>Good</th></tr></thead>
  <tbody>
  <tr><td>

  ```go
  mu := new(sync.Mutex)
  mu.Lock()
  ```

  </td><td>

  ```go
  var mu sync.Mutex
  mu.Lock()
  ```

  </td></tr>
  </tbody></table>

- For the same reason, a struct field of `sync.Mutex` does not require explicit initialization.

  <table>
  <thead><tr><th>Bad</th><th>Good</th></tr></thead>
  <tbody>
  <tr><td>

  ```go
  type Store struct {
    mu sync.Mutex
  }

  s := Store{
    mu: sync.Mutex{},
  }

  // use s
  ```

  </td><td>

  ```go
  type Store struct {
    mu sync.Mutext
  }

  var s Store



  // use s
  ```

  </td></tr>
  </tbody></table>

- The zero value (a slice declared with `var`) is usable immediately without `make()`.

  <table>
  <thead><tr><th>Bad</th><th>Good</th></tr></thead>
  <tbody>
  <tr><td>

  ```go
  nums := []int{}
  // or, nums := make([]int)

  if add1 {
    nums = append(nums, 1)
  }

  if add2 {
    nums = append(nums, 2)
  }
  ```

  </td><td>

  ```go
  var nums []int

  if add1 {
    nums = append(nums, 1)
  }

  if add2 {
    nums = append(nums, 2)
  }

  ```

  </td></tr>
  </tbody></table>

`nil` is a valid slice of length 0. This means that,

- You should not return a slice of length zero explicitly. Return `nil` instead.

  <table>
  <thead><tr><th>Bad</th><th>Good</th></tr></thead>
  <tbody>
  <tr><td>

  ```go
  if x == "" {
    return []int{}
  }
  ```

  </td><td>

  ```go
  if x == "" {
    return nil
  }
  ```

  </td></tr>
  </tbody></table>
*Note:* in the case of serialization it might make sense to use a zero length initialized slice. For instance the JSON representation of a _nil_ slice is `null`, however a zero length allocated slice would be translated to `[]`.

- To check if a slice is empty, always use `len(s) == 0`. Do not check for `nil`.

  <table>
  <thead><tr><th>Bad</th><th>Good</th></tr></thead>
  <tbody>
  <tr><td>

  ```go
  func isEmpty(s []string) bool {
    return s == nil
  }
  ```

  </td><td>

  ```go
  func isEmpty(s []string) bool {
    return len(s) == 0
  }
  ```

  </td></tr>
  </tbody></table>

Remember that, while it is a valid slice, a `nil` slice is not equivalent to an allocated slice of length `0` - one is `nil` and the other is not - and the two may be treated differently in different situations (such as serialization and comparison).

## Beware of Copying Mutexes in Go

The `sync.Mutex` is a value type so copying it is wrong. We're just creating a different mutex, so obviously the exclusion no longer works.

<table>
<thead><tr><th>Bad</th> <th>Good</th></tr></thead>
<tbody>
<tr>
<td>

```go
type Container struct {
  mu sync.Mutex
  counters map[string]int
}

func (c Container) inc(name string) { // the value receiver will make a copy of the mutex
  c.mu.Lock()
  defer c.mu.Unlock()
  c.counters[name]++
}
```

</td>
<td>

```go
type Container struct {
  my.sync.Mutex
  counters map[string]int
}

func (c *Container) inc(name string) {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.counters[name]++
}
```

</td>
</tr>

</tbody>
</table>

## Copy Slices and Maps at Boundaries

Slices and maps contain pointers to the underlying data so be wary of scenarios when they need to be copied.

### Receiving Slices and Maps

Keep in mind that users can modify a map or slice you received as an argument if you store a reference to it.

<table>
<thead><tr><th>Bad</th> <th>Good</th></tr></thead>
<tbody>
<tr>
<td>

```go
func (d *Driver) SetTrips(trips []Trip) {
  d.trips = trips
}

trips := ...
d1.SetTrips(trips)

// Did you mean to modify d1.trips?
trips[0] = ...
```

</td>
<td>

```go
func (d *Driver) SetTrips(trips []Trip) {
  d.trips = make([]Trip, len(trips))
  copy(d.trips, trips)
}

trips := ...
d1.SetTrips(trips)

// We can now modify trips[0] without affecting d1.trips.
trips[0] = ...
```

</td>
</tr>

</tbody>
</table>

### Returning Slices and Maps

Similarly, be wary of user modifications to maps or slices exposing internal state.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
type Stats struct {
  mu sync.Mutex
  counters map[string]int
}

// Snapshot returns the current stats.
func (s *Stats) Snapshot() map[string]int {
  s.mu.Lock()
  defer s.mu.Unlock()

  return s.counters
}



// snapshot is no longer protected by the mutex, so any
// access to the snapshot is subject to data races.
snapshot := stats.Snapshot()
```

</td><td>

```go
type Stats struct {
  mu sync.Mutex
  counters map[string]int
}

func (s *Stats) Snapshot() map[string]int {
  s.mu.Lock()
  defer s.mu.Unlock()

  result := make(map[string]int, len(s.counters))
  for k, v := range s.counters {
    result[k] = v
  }
  return result
}

// Snapshot is now a copy.
snapshot := stats.Snapshot()
```

</td></tr>
</tbody></table>

### Filtering in place

This trick uses the fact that a slice shares the same backing array and capacity as the original, so the storage is reused for the filtered slice. Of course, the original contents are modified, so be mindful. It is useful for the code in the 'hot path' where we want to minimize allocation.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
var b []rune
for _, x := range a {
  if f(x) {
    b = append(b, x) // will cause new allocations
  }
}










```

</td><td>

```go
b := a[:0]
for _, x := range a {
  if f(x) {
    b = append(b, x)  // will reuse the backing array
  }
}

// alternatively using index, a bit more verbose
n := 0
for _, x := range a {
  if f(x) {
    a[n] = x
    n++
  }
}
a = a[:n]
```

</td></tr>
</tbody></table>

## Pointers to Interfaces

You almost never need a pointer to an interface. You should be passing interfaces as values—the underlying data can still be a pointer.

### Verify Interface Compliance

Verify interface compliance at compile time *where appropriate*. This includes:

- Exported types that are required to implement specific interfaces as part of their API contract
- Exported or unexported types that are part of a collection of types implementing the same interface
- Other cases where violating an interface would break users

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
type Handler struct {
  // ...
}



func (h *Handler) ServeHTTP(
  w http.ResponseWriter,
  r *http.Request,
) {
  ...
}
```

</td><td>

```go
type Handler struct {
  // ...
}

var _ http.Handler = (*Handler)(nil)

func (h *Handler) ServeHTTP(
  w http.ResponseWriter,
  r *http.Request,
) {
  // ...
}
```

</td></tr>
</tbody></table>

The statement `var _ http.Handler = (*Handler)(nil)` will fail to compile if `*Handler` ever stops matching the `http.Handler` interface.

The right hand side of the assignment should be the zero value of the asserted type. This is `nil` for pointer types (like `*Handler`), slices, and maps, and an empty struct for struct types.

```go
type LogHandler struct {
  h   http.Handler
  log *zap.Logger
}

var _ http.Handler = LogHandler{}

func (h LogHandler) ServeHTTP(
  w http.ResponseWriter,
  r *http.Request,
) {
  // ...
}
```

## Receivers and Interfaces

Methods with value receivers can be called on pointers as well as values.
Methods with pointer receivers can only be called on pointers or [addressable values].

  [addressable values]: https://golang.org/ref/spec#Method_values

For example,

```go
type S struct {
  data string
}

func (s S) Read() string {
  return s.data
}

func (s *S) Write(str string) {
  s.data = str
}

sVals := map[int]S{1: {"A"}}

// You can only call Read using a value
sVals[1].Read()

// This will not compile:
//  sVals[1].Write("test")

sPtrs := map[int]*S{1: {"A"}}

// You can call both Read and Write using a pointer
sPtrs[1].Read()
sPtrs[1].Write("test")
```

Similarly, an interface can be satisfied by a pointer, even if the method has a value receiver.

```go
type F interface {
  f()
}

type S1 struct{}

func (s S1) f() {}

type S2 struct{}

func (s *S2) f() {}

s1Val := S1{}
s1Ptr := &S1{}
s2Val := S2{}
s2Ptr := &S2{}

var i F
i = s1Val
i = s1Ptr
i = s2Ptr

// The following doesn't compile, since s2Val is a value, and there is no value receiver for f.
//   i = s2Val
```

Effective Go has a good write up on [Pointers vs. Values].

  [Pointers vs. Values]: https://golang.org/doc/effective_go.html#pointers_vs_values

## Defer to Clean Up

Use defer to clean up resources such as files and locks.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
p.Lock()
if p.count < 10 {
  p.Unlock()
  return p.count
}

p.count++
newCount := p.count
p.Unlock()

return newCount

// easy to miss unlocks due to multiple returns
```

</td><td>

```go
p.Lock()
defer p.Unlock()

if p.count < 10 {
  return p.count
}

p.count++
return p.count



// more readable
```

</td></tr>
</tbody></table>

Defer has an extremely small overhead and should be avoided only if you can prove that your function execution time is in the order of nanoseconds. The readability win of using defers is worth the miniscule cost of using them. This is especially true for larger methods that have more than simple memory accesses, where the other computations are more significant than the `defer`.

## Channel Size is One or None

Channels should usually have a size of one or be unbuffered. By default, channels are unbuffered and have a size of zero (blocking behaviour). Any other size must be subject to a high level of scrutiny. Consider how the size is determined, what prevents the channel from filling up under load and blocking writers, and what happens when this occurs.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// Ought to be enough for anybody!
c := make(chan int, 64)


```

</td><td>

```go
// Size of one
c := make(chan int, 1) // or
// Unbuffered channel, size of zero
c := make(chan int)
```

</td></tr>
</tbody></table>

*Note:* in some places we use buffered channels to implement a semaphore as described [here](https://robreid.io/stupid-channel-tricks-p2-semaphores/)

## Start Enums at One

The standard way of introducing enumerations in Go is to declare a custom type and a `const` group with `iota`. Since variables have a 0 default value, you should usually start your enums on a non-zero value.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
type Operation int

const (
  Add Operation = iota
  Subtract
  Multiply
)

// Add=0, Subtract=1, Multiply=2
```

</td><td>

```go
type Operation int

const (
  Add Operation = iota + 1
  Subtract
  Multiply
)

// Add=1, Subtract=2, Multiply=3
```

</td></tr>
</tbody></table>

*Note:* There are cases where using the zero value makes sense, for example when the zero value case is the desirable default behavior.

```go
type LogOutput int

const (
  LogToStdout LogOutput = iota
  LogToFile
  LogToRemote
)

// LogToStdout=0, LogToFile=1, LogToRemote=2
```

## Use `"time"` to handle time

Time is complicated. Incorrect assumptions often made about time include the following.

1. A day has 24 hours
2. An hour has 60 minutes
3. A week has 7 days
4. A year has 365 days
5. [And a lot more](https://infiniteundo.com/post/25326999628/falsehoods-programmers-believe-about-time)

For example, *1* means that adding 24 hours to a time instant will not always yield a new calendar day.

Therefore, always use the [`"time"`] package when dealing with time because it helps deal with these incorrect assumptions in a safer, more accurate manner.

  [`"time"`]: https://golang.org/pkg/time/

### Use `time.Time` for instants of time

Use [`time.Time`] when dealing with instants of time, and the methods on `time.Time` when comparing, adding, or subtracting time.

  [`time.Time`]: https://golang.org/pkg/time/#Time

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
func isActive(now, start, stop int) bool {
  return start <= now && now < stop
}
```

</td><td>

```go
func isActive(now, start, stop time.Time) bool {
  return (start.Before(now) || start.Equal(now)) && now.Before(stop)
}
```

</td></tr>
</tbody></table>

### Use `time.Duration` for periods of time

Use [`time.Duration`] when dealing with periods of time.

  [`time.Duration`]: https://golang.org/pkg/time/#Duration

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
func poll(delay int) {
  for {
    // ...
    time.Sleep(time.Duration(delay) * time.Millisecond)
  }
}

poll(10) // was it seconds or milliseconds?
```

</td><td>

```go
func poll(delay time.Duration) {
  for {
    // ...
    time.Sleep(delay)
  }
}

poll(10*time.Second)
```

</td></tr>
</tbody></table>

Going back to the example of adding 24 hours to a time instant, the method we use to add time depends on intent. If we want the same time of the day, but on the next calendar day, we should use [`Time.AddDate`]. However, if we want an instant of time guaranteed to be 24 hours after the previous time, we should use [`Time.Add`].

  [`Time.AddDate`]: https://golang.org/pkg/time/#Time.AddDate
  [`Time.Add`]: https://golang.org/pkg/time/#Time.Add

```go
newDay := t.AddDate(0 /* years */, 0 /* months */, 1 /* days */)
maybeNewDay := t.Add(24 * time.Hour)
```

### Use `time.Time` and `time.Duration` with external systems

Use `time.Duration` and `time.Time` in interactions with external systems when possible. For example:

- Command-line flags: [`flag`] supports `time.Duration` via [`time.ParseDuration`]
- JSON: [`encoding/json`] supports encoding `time.Time` as an [RFC 3339] string via its [`UnmarshalJSON` method]
- SQL: [`database/sql`] supports converting `DATETIME` or `TIMESTAMP` columns into `time.Time` and back if the underlying driver supports it
- YAML: [`gopkg.in/yaml.v2`] supports `time.Time` as an [RFC 3339] string, and `time.Duration` via [`time.ParseDuration`].

  [`flag`]: https://golang.org/pkg/flag/
  [`time.ParseDuration`]: https://golang.org/pkg/time/#ParseDuration
  [`encoding/json`]: https://golang.org/pkg/encoding/json/
  [RFC 3339]: https://tools.ietf.org/html/rfc3339
  [`UnmarshalJSON` method]: https://golang.org/pkg/time/#Time.UnmarshalJSON
  [`database/sql`]: https://golang.org/pkg/database/sql/
  [`gopkg.in/yaml.v2`]: https://godoc.org/gopkg.in/yaml.v2

When it is not possible to use `time.Duration` in these interactions, use `int` or `float64` and include the unit in the name of the field. For example, since `encoding/json` does not support `time.Duration`, the unit is included in the name of the field.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// {"interval": 2}
type Config struct {
  Interval int `json:"interval"`
}
```

</td><td>

```go
// {"intervalMillis": 2000}
type Config struct {
  IntervalMillis int `json:"intervalMillis"`
}
```

</td></tr>
</tbody></table>

When it is not possible to use `time.Time` in these interactions, unless an alternative is agreed upon, use `string` and format timestamps as defined in [RFC 3339]. This format is used by default by [`Time.nmarshalText`] and is available for use in `Time.Format` and `time.Parse` via [`time.RFC3339`].

  [`Time.UnmarshalText`]: https://golang.org/pkg/time/#Time.UnmarshalText
  [`time.RFC3339`]: https://golang.org/pkg/time/#RFC3339

Although this tends to not be a problem in practice, keep in mind that the `"time"` package does not support parsing timestamps with leap seconds ([8728]), nor does it account for leap seconds in calculations ([15190]). If you compare two instants of time, the difference will not include the leap seconds that may have occurred between those two instants.

  [8728]: https://github.com/golang/go/issues/8728
  [15190]: https://github.com/golang/go/issues/15190

## Error Types

There are various options for declaring errors:

- [`errors.New`] for errors with simple static strings
- [`fmt.Errorf`] for formatted error strings
- Custom types that implement an `Error()` method
- Wrapped errors using [`"pkg/errors".Wrap`]

When returning errors, consider the following to determine the best choice:

- Is this a simple error that needs no extra information? If so, [`errors.New`] should suffice.
- Do the clients need to detect and handle this error? If so, you should use a custom type, and implement the `Error()` method.
- Are you propagating an error returned by a downstream function? If so, check the [section on error wrapping](#error-wrapping).
- Otherwise, [`fmt.Errorf`] is okay.

  [`errors.New`]: https://golang.org/pkg/errors/#New
  [`fmt.Errorf`]: https://golang.org/pkg/fmt/#Errorf
  [`"pkg/errors".Wrap`]: https://godoc.org/github.com/pkg/errors#Wrap

If the client needs to detect the error, and you have created a simple error
using [`errors.New`], use a var for the error.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// package foo

func Open() error {
  return errors.New("could not open")
}

// package bar

func use() {
  if err := foo.Open(); err != nil {
    if err.Error() == "could not open" {
      // handle
    } else {
      panic("unknown error")
    }
  }
}
```

</td><td>

```go
// package foo

var ErrCouldNotOpen = errors.New("could not open")

func Open() error {
  return ErrCouldNotOpen
}

// package bar

if err := foo.Open(); err != nil {
  if errors.Is(err, foo.ErrCouldNotOpen) {
    // handle
  } else {
    panic("unknown error")
  }
}
```

</td></tr>
</tbody></table>

If you have an error that clients may need to detect, and you would like to add more information to it (e.g., it is not a static string), then you should use a custom type.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
func open(file string) error {
  return fmt.Errorf("file %q not found", file)
}

func use() {
  if err := open("testfile.txt"); err != nil {
    if strings.Contains(err.Error(), "not found") {
      // handle
    } else {
      panic("unknown error")
    }
  }
}








```

</td><td>

```go
type errNotFound struct {
  file string
}

func (e errNotFound) Error() string {
  return fmt.Sprintf("file %q not found", e.file)
}

func open(file string) error {
  return errNotFound{file: file}
}

func use() {
  if err := open("testfile.txt"); err != nil {
    if _, ok := err.(errNotFound); ok {
      // handle
    } else {
      panic("unknown error")
    }
  }
}
```

</td></tr>
</tbody></table>

Be careful with exporting custom error types directly since they become part of the public API of the package. It is preferable to expose matcher functions to check the error instead.

```go
// package foo

type errNotFound struct {
  file string
}

func (e errNotFound) Error() string {
  return fmt.Sprintf("file %q not found", e.file)
}

func IsNotFoundError(err error) bool {
  return errors.Is(err, errNotFound)
}

func Open(file string) error {
  return errNotFound{file: file}
}

// package bar

if err := foo.Open("foo"); err != nil {
  if foo.IsNotFoundError(err) {
    // handle
  } else {
    panic("unknown error")
  }
}
```

### Error Wrapping

There are three main options for propagating errors if a call fails:

- Return the original error if there is no additional context to add and you want to maintain the original error type.
- Use [`fmt.Errorf`] if the callers do not need to detect or handle that specific error case.
- Use a custom type where you can detail the failure reason.

It is recommended to add context where possible so that instead of a vague error such as "connection refused", you get more useful errors such as "call service foo: connection refused". When adding context to returned errors, keep the context succinct by avoiding phrases like "failed to", which state the obvious and pile up as the error percolates up through the stack:

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
s, err := store.New()
if err != nil {
    return fmt.Errorf("failed to create new store: %v", err)
}
```

</td><td>

```go
s, err := store.New()
if err != nil {
    return fmt.Errorf("new store: %v", err)
}
```

<tr><td>

```
failed to x: failed to y: failed to create new store: the error
```

</td><td>

```
x: y: new store: the error

```

</td></tr>
</tbody></table>

However once the error is sent to another system, it should be clear the message is an error (e.g. an `err` tag or "Failed" prefix in logs).

See also [Don't just check errors, handle them gracefully].

  [`"pkg/errors".Cause`]: https://godoc.org/github.com/pkg/errors#Cause
  [Don't just check errors, handle them gracefully]: https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully

### Handle Type Assertion Failures

The single return value form of a [type assertion] will panic on an incorrect type. Therefore, always use the "comma ok" idiom.

  [type assertion]: https://golang.org/ref/spec#Type_assertions

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
t := i.(string)



```

</td><td>

```go
t, ok := i.(string)
if !ok {
  // handle the error gracefully
}
```

</td></tr>
</tbody></table>

### Panic with care

Code running in production must avoid panics. Panics are a major source of [cascading failures]. If an error occurs, the function must return an error and allow the caller to decide how to handle it.

  [cascading failures]: https://en.wikipedia.org/wiki/Cascading_failure

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
func run(args []string) {
  if len(args) == 0 {
    panic("an argument is required")
  }
  // ...
}

func main() {
  run(os.Args[1:])
}




```

</td><td>

```go
func run(args []string) error {
  if len(args) == 0 {
    return errors.New("an argument is required")
  }
  // ...
  return nil
}

func main() {
  if err := run(os.Args[1:]); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
}
```

</td></tr>
</tbody></table>

Panic/recover is not an error handling strategy. A program must panic only when something irrecoverable happens such as a nil dereference. An exception to this is program initialization: bad things at program startup that should abort the program may cause panic.

```go
var _statusTemplate = template.Must(template.New("name").Parse("_statusHTML"))
```

Even in tests, prefer `t.Fatal` or `t.FailNow` over panics to ensure that the test is marked as failed.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// func TestFoo(t *testing.T)

f, err := os.CreateTemp("", "test")
if err != nil {
  panic("failed to set up test")
}
```

</td><td>

```go
// func TestFoo(t *testing.T)

f, err := os.CreateTemp("", "test")
if err != nil {
  t.Fatal("failed to set up test")
}
```

</td></tr>
</tbody></table>

## Avoid Mutable Globals

Avoid mutating global variables, instead opting for dependency injection. This applies to function pointers as well as other kinds of values.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// sign.go

var _timeNow = time.Now

func sign(msg string) string {
  now := _timeNow()
  return signWithTime(msg, now)
}








```

</td><td>

```go
// sign.go

type signer struct {
  now func() time.Time
}

func newSigner() *signer {
  return &signer{
    now: time.Now,
  }
}

func (s *signer) Sign(msg string) string {
  now := s.now()
  return signWithTime(msg, now)
}
```
</td></tr>
<tr><td>

```go
// sign_test.go

func TestSign(t *testing.T) {
  oldTimeNow := _timeNow
  _timeNow = func() time.Time {
    return someFixedTime
  }
  defer func() { _timeNow = oldTimeNow }()

  assert.Equal(t, want, sign(give))
}
```

</td><td>

```go
// sign_test.go

func TestSigner(t *testing.T) {
  s := newSigner()
  s.now = func() time.Time {
    return someFixedTime
  }

  assert.Equal(t, want, s.Sign(give))
}

```

</td></tr>
</tbody></table>

## Avoid Embedding Types in Public Structs

These embedded types leak implementation details, inhibit type evolution, and obscure documentation. Assuming you have implemented a variety of list types using a shared `AbstractList`, avoid embedding the `AbstractList` in your concrete list implementations. Instead, hand-write only the methods to your concrete list that will delegate to the abstract list.

```go
type AbstractList struct {}

// Add adds an entity to the list.
func (l *AbstractList) Add(e Entity) {
  // ...
}

// Remove removes an entity from the list.
func (l *AbstractList) Remove(e Entity) {
  // ...
}
```

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// ConcreteList is a list of entities.
type ConcreteList struct {
  *AbstractList
}










```

</td><td>

```go
// ConcreteList is a list of entities.
type ConcreteList struct {
  list *AbstractList
}

// Add adds an entity to the list.
func (l *ConcreteList) Add(e Entity) {
  l.list.Add(e)
}

// Remove removes an entity from the list.
func (l *ConcreteList) Remove(e Entity) {
  l.list.Remove(e)
}
```

</td></tr>
</tbody></table>

Go allows [type embedding] as a compromise between inheritance and composition. The outer type gets implicit copies of the embedded type's methods. These methods, by default, delegate to the same method of the embedded instance.

  [type embedding]: https://golang.org/doc/effective_go.html#embedding

The struct also gains a field by the same name as the type. So, if the embedded type is public, the field is public. To maintain backward compatibility, every future version of the outer type must keep the embedded type.

An embedded type is rarely necessary. It is a convenience that helps you avoid writing tedious delegate methods.

Even embedding a compatible AbstractList *interface*, instead of the struct, would offer the developer more flexibility to change in the future, but still leak the detail that the concrete lists use an abstract implementation.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
// AbstractList is a generalized implementation
// for various kinds of lists of entities.
type AbstractList interface {
  Add(Entity)
  Remove(Entity)
}

// ConcreteList is a list of entities.
type ConcreteList struct {
  AbstractList
}




```

</td><td>

```go
// ConcreteList is a list of entities.
type ConcreteList struct {
  list AbstractList
}

// Add adds an entity to the list.
func (l *ConcreteList) Add(e Entity) {
  l.list.Add(e)
}

// Remove removes an entity from the list.
func (l *ConcreteList) Remove(e Entity) {
  l.list.Remove(e)
}
```

</td></tr>
</tbody></table>

Either with an embedded struct or an embedded interface, the embedded type places limits on the evolution of the type.

- Adding methods to an embedded interface is a breaking change.
- Removing methods from an embedded struct is a breaking change.
- Removing the embedded type is a breaking change.
- Replacing the embedded type, even with an alternative that satisfies the same
  interface, is a breaking change.

Although writing these delegate methods is tedious, the additional effort hides an implementation detail, leaves more opportunities for change and also eliminates indirection for discovering the full List interface in documentation.

## Avoid Using Built-In Names

The Go [language specification] outlines several built-in, [predeclared identifiers] that should not be used as names within Go programs.

Depending on context, reusing these identifiers as names will either shadow the original within the current lexical scope (and any nested scopes) or make affected code confusing. In the best case, the compiler will complain; in the worst case, such code may introduce latent, hard-to-grep bugs.

  [language specification]: https://golang.org/ref/spec
  [predeclared identifiers]: https://golang.org/ref/spec#Predeclared_identifiers

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
var error string
// `error` shadows the builtin

// or

func handleErrorMessage(error string) {
    // `error` shadows the builtin
}
```

</td><td>

```go
var errorMessage string
// `error` refers to the builtin

// or

func handleErrorMessage(msg string) {
    // `error` refers to the builtin
}
```

</td></tr>
<tr><td>

```go
type Foo struct {
    // While these fields technically don't
    // constitute shadowing, grepping for
    // `error` or `string` strings is now
    // ambiguous.
    error  error
    string string
}

func (f Foo) Error() error {
    // `error` and `f.error` are
    // visually similar
    return f.error
}

func (f Foo) String() string {
    // `string` and `f.string` are
    // visually similar
    return f.string
}
```

</td><td>

```go
type Foo struct {
    // `error` and `string` strings are
    // now unambiguous.
    err error
    str string
}

func (f Foo) Error() error {
    return f.err
}

func (f Foo) String() string {
    return f.str
}






```
</td></tr>
</tbody></table>

*Note:* the compiler will not generate errors when using predeclared identifiers, but tools such as `go vet` should correctly point out these and other cases of shadowing.

## Avoid `init()`

Avoid `init()` where possible. When `init()` is unavoidable or desirable, code should attempt to:

1. Be completely deterministic, regardless of program environment or invocation.
2. Avoid depending on the ordering or side-effects of other `init()` functions. While `init()` ordering is well-known, code can change, and thus relationships between `init()` functions can make code brittle and   error-prone.
3. Avoid accessing or manipulating global or environment state, such as machine information, environment variables, working directory, program arguments/inputs, etc.
4. Avoid I/O, including both filesystem, network, and system calls.

Code that cannot satisfy these requirements likely belongs as a helper to be called as part of `main()` (or elsewhere in a program's lifecycle), or be written as part of `main()` itself. In particular, libraries that are intended to be used by other programs should take special care to be completely deterministic and not perform "init magic".

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
type Foo struct {
    // ...
}

var _defaultFoo Foo

func init() {
    _defaultFoo = Foo{
        // ...
    }
}


```

</td><td>

```go
var _defaultFoo = Foo{
    // ...
}

// or, better, for testability:

var _defaultFoo = defaultFoo()

func defaultFoo() Foo {
    return Foo{
        // ...
    }
}
```

</td></tr>
<tr><td>

```go
type Config struct {
    // ...
}

var _config Config

func init() {
    // Bad: based on current directory
    cwd, _ := os.Getwd()

    // Bad: I/O
    raw, _ := os.ReadFile(
        path.Join(cwd, "config", "config.yaml"),
    )

    yaml.Unmarshal(raw, &_config)
}
```

</td><td>

```go
type Config struct {
    // ...
}

func loadConfig() Config {
    cwd, err := os.Getwd()
    // handle err

    raw, err := os.ReadFile(
        path.Join(cwd, "config", "config.yaml"),
    )
    // handle err

    var config Config
    yaml.Unmarshal(raw, &config)

    return config
}
```

</td></tr>
</tbody></table>

Considering the above, some situations in which `init()` may be preferable or necessary might include:

- Complex expressions that cannot be represented as single assignments.
- Pluggable hooks, such as `database/sql` dialects, encoding type registries, etc.
- Optimizations to [Google Cloud Functions] and other forms of deterministic precomputation.

  [Google Cloud Functions]: https://cloud.google.com/functions/docs/bestpractices/tips#use_global_variables_to_reuse_objects_in_future_invocations

## Exit in Main

Go programs use [`os.Exit`] or [`log.Fatal*`] to exit immediately. (Panicking is not a good way to exit programs, please [Panic with care](#panic-with-care).)

  [`os.Exit`]: https://golang.org/pkg/os/#Exit
  [`log.Fatal*`]: https://golang.org/pkg/log/#Fatal

Call one of `os.Exit` or `log.Fatal*` **only in `main()`**. All other functions should return errors to signal failure.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
func main() {
  body := readFile(path)
  fmt.Println(body)
}

func readFile(path string) string {
  f, err := os.Open(path)
  if err != nil {
    log.Fatal(err)
  }

  b, err := io.ReadAll(f)
  if err != nil {
    log.Fatal(err)
  }

  return string(b)
}



```

</td><td>

```go
func main() {
  body, err := readFile(path)
  if err != nil {
    log.Fatal(err)
  }
  fmt.Println(body)
}

func readFile(path string) (string, error) {
  f, err := os.Open(path)
  if err != nil {
    return "", err
  }

  b, err := io.ReadAll(f)
  if err != nil {
    return "", err
  }

  return string(b), nil
}
```

</td></tr>
</tbody></table>

Rationale: Programs with multiple functions that exit present a few issues:

- Non-obvious control flow: Any function can exit the program so it becomes difficult to reason about the control flow.
- Difficult to test: A function that exits the program will also exit the test calling it. This makes the function difficult to test and introduces risk of skipping other tests that have not yet been run by `go test`.
- Skipped cleanup: When a function exits the program, it skips function calls enqueued with `defer` statements. This adds risk of skipping important cleanup tasks.

### Exit Once

If possible, prefer to call `os.Exit` or `log.Fatal` **at most once** in your `main()`. If there are multiple error scenarios that halt program execution, put that logic under a separate function and return errors from it.

This has the effect of shortening your `main()` function and putting all key business logic into a separate, testable function.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
package main

func main() {
  args := os.Args[1:]
  if len(args) != 1 {
    log.Fatal("missing file")
  }
  name := args[0]

  f, err := os.Open(name)
  if err != nil {
    log.Fatal(err)
  }
  defer f.Close()

  // If we call log.Fatal after this line,
  // f.Close will not be called.

  b, err := io.ReadAll(f)
  if err != nil {
    log.Fatal(err)
  }

  // ...
}



```

</td><td>

```go
package main

func main() {
  if err := run(); err != nil {
    log.Fatal(err)
  }
}

func run() error {
  args := os.Args[1:]
  if len(args) != 1 {
    return errors.New("missing file")
  }
  name := args[0]

  f, err := os.Open(name)
  if err != nil {
    return err
  }
  defer f.Close()

  b, err := io.ReadAll(f)
  if err != nil {
    return err
  }

  // ...
}
```

</td></tr>
</tbody></table>

## Performance

Performance-specific guidelines apply only to the hot path.

### Prefer strconv over fmt

When converting primitives to/from strings, `strconv` is faster than `fmt`.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
for i := 0; i < b.N; i++ {
  s := fmt.Sprint(rand.Int())
}
```

</td><td>

```go
for i := 0; i < b.N; i++ {
  s := strconv.Itoa(rand.Int())
}
```

</td></tr>
<tr><td>

```
BenchmarkFmtSprint-4    143 ns/op    2 allocs/op
```

</td><td>

```
BenchmarkStrconv-4    64.2 ns/op    1 allocs/op
```

</td></tr>
</tbody></table>

### Avoid string-to-byte conversion

Do not create byte slices from a fixed string repeatedly. Instead, perform the conversion once and capture the result.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
for i := 0; i < b.N; i++ {
  w.Write([]byte("Hello world"))
}

```

</td><td>

```go
data := []byte("Hello world")
for i := 0; i < b.N; i++ {
  w.Write(data)
}
```

</tr>
<tr><td>

```
BenchmarkBad-4   50000000   22.2 ns/op
```

</td><td>

```
BenchmarkGood-4  500000000   3.25 ns/op
```

</td></tr>
</tbody></table>

### Prefer Specifying Container Capacity

Specify container capacity where possible in order to allocate memory for the container up front. This minimizes subsequent allocations (by copying and resizing of the container) as elements are added.

### Specifying Map Capacity Hints

Where possible, provide capacity hints when initializing maps with `make()`.

```go
make(map[T1]T2, hint)
```

Providing a capacity hint to `make()` tries to right-size the map at initialization time, which reduces the need for growing the map and allocations as elements are added to the map.

*Note:* unlike slices, map capacity hints do not guarantee complete, preemptive allocation, but are used to approximate the number of hashmap buckets required. Consequently, allocations may still occur when adding elements to the map, even up to the specified capacity.

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
m := make(map[string]os.FileInfo)

files, _ := os.ReadDir("./files")
for _, f := range files {
    m[f.Name()] = f
}

```

</td><td>

```go

files, _ := os.ReadDir("./files")

m := make(map[string]os.FileInfo, len(files))
for _, f := range files {
    m[f.Name()] = f
}
```

</td></tr>
<tr><td>

`m` is created without a size hint; there may be more allocations at assignment time.

</td><td>

`m` is created with a size hint; there may be fewer allocations at assignment time.

</td></tr>
</tbody></table>

### Specifying Slice Capacity

Where possible, provide capacity hints when initializing slices with `make()`, particularly when appending.

```go
make([]T, length, capacity)
```

Unlike maps, slice capacity is not a hint: the compiler will allocate enough memory for the capacity of the slice as provided to `make()`, which means that subsequent `append()` operations will incur zero allocations (until the length of the slice matches the capacity, after which any appends will require a resize to hold additional elements).

<table>
<thead><tr><th>Bad</th><th>Good</th></tr></thead>
<tbody>
<tr><td>

```go
for n := 0; n < b.N; n++ {
  data := make([]int, 0)
  for k := 0; k < size; k++{
    data = append(data, k)
  }
}
```

</td><td>

```go
for n := 0; n < b.N; n++ {
  data := make([]int, 0, size)
  for k := 0; k < size; k++{
    data = append(data, k)
  }
}
```

</td></tr>
<tr><td>

```
BenchmarkBad-4    100000000    2.48s
```

</td><td>

```
BenchmarkGood-4   100000000    0.21s
```

</td></tr>
</tbody></table>
