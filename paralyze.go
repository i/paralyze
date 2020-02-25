package paralyze

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Paralyzable is a type of function that can be paralyzed. Since most
// functions don't carry this signature, a common pattern is to wrap an
// existing function in a Paralyzable function.
type Paralyzable func() (interface{}, error)

// ParalyzableCtx is the same as a Paralyzable function, except it accepts a
// context.Context. Functions that implement this type should respect the
// documentation found here: https://godoc.org/context
type ParalyzableCtx func(context.Context) (interface{}, error)

// General errors that can be returned from the paralyze package
var (
	ErrTimedOut = errors.New("timed out")
	ErrCanceled = errors.New("canceled")
)

type ErrPanic struct{ panik interface{} }

func (e *ErrPanic) Error() string { return "panicked" }

// Paralyze parallelizes a function and returns a slice containing results and
// a slice containing errors. The results at each index are not mutually exclusive,
// that is if results[i] is not nil, errors[i] is not guaranteed to be nil.
func Paralyze(funcs ...Paralyzable) (results []interface{}, errors []error) {
	return NewParalyzer().Do(funcs...)
}

type ResErr struct {
	Res interface{}
	Err error
}

// ParalyzeM parallelizes a map of strings to functions. The return type is a
// map of keys to a map containing two keys: res and err.
func ParalyzeM(m map[string]Paralyzable) map[string]ResErr {
	var names []string
	var fns []Paralyzable

	for name, fn := range m {
		names = append(names, name)
		fns = append(fns, fn)
	}

	res := make(map[string]ResErr)
	results, errs := Paralyze(fns...)
	for i := range results {
		name := names[i]
		res[name] = ResErr{
			Res: results[i],
			Err: errs[i],
		}
	}

	return res
}

// ParalyzeWithTimeout does the same as Paralyze, but it accepts a timeout. If
// the timeout is exceeded before all paralyzed functions are complete, the
// unfinished results will be discarded without being cancelled. Any complete
// tasks will be unaffected.
func ParalyzeWithTimeout(timeout time.Duration, funcs ...Paralyzable) ([]interface{}, []error) {
	if timeout == 0 {
		return Paralyze(funcs...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	results, errors := NewParalyzer().DoContext(ctx, funcs...)
	for i, err := range errors {
		if err == context.DeadlineExceeded {
			errors[i] = ErrTimedOut
		}
	}

	return results, errors
}

// ParalyzeWithCancel does the same as Paralyze, but it accepts a channel that
// allows the function to respond before the paralyzed functions are finished.
// Any functions that are still oustanding will have errors set as ErrCanceled.
func ParalyzeWithCancel(done <-chan struct{}, funcs ...Paralyzable) ([]interface{}, []error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-done:
				cancel()
			}
		}
	}()

	results, errors := NewParalyzer().DoContext(ctx, funcs...)
	for i, err := range errors {
		if err == context.Canceled {
			errors[i] = ErrCanceled
		}
	}

	return results, errors
}

// ParalyzeWithContext takes a slice of functions that accept a
// context.Context. These functions are responsible for releasing resources
// (closing connections, etc.) and should respect ctx.Done().
func ParalyzeWithContext(ctx context.Context, funcs ...ParalyzableCtx) ([]interface{}, []error) {
	paralyzed := make([]Paralyzable, 0, len(funcs))
	for _, f := range funcs {
		f := f // Copy
		paralyzed = append(paralyzed, func() (interface{}, error) { return f(ctx) })
	}

	return NewParalyzer().Do(paralyzed...)
}

func ParalyzeLimit(limit int, tasks ...Paralyzable) ([]interface{}, []error) {
	return NewParalyzer(WithConcurrencyLimit(limit)).Do(tasks...)
}

type Paralyzer struct {
	concurrency int
}

type Option func(p *Paralyzer) *Paralyzer

func WithConcurrencyLimit(n int) Option {
	return func(p *Paralyzer) *Paralyzer {
		p.concurrency = n
		return p
	}
}

func NewParalyzer(opts ...Option) *Paralyzer {
	p := new(Paralyzer)
	for _, opt := range opts {
		p = opt(p)
	}
	return p
}

func (p *Paralyzer) Do(funcs ...Paralyzable) ([]interface{}, []error) {
	return p.DoContext(context.Background(), funcs...)
}

func convert(fn Paralyzable) func() chan ResErr {
	return func() chan ResErr {
		ch := make(chan ResErr, 1)

		go func() {
			var res interface{}
			var err error

			defer func() {
				if r := recover(); r != nil {
					ch <- ResErr{nil, &ErrPanic{r}}
				}
			}()

			res, err = fn()
			ch <- ResErr{res, err}
		}()

		return ch
	}
}

func (p *Paralyzer) DoContext(
	ctx context.Context,
	funcs ...Paralyzable,
) ([]interface{}, []error) {
	var wg sync.WaitGroup
	var sem chan struct{}

	var panik interface{}
	var panikOnce sync.Once

	var numFuncs = len(funcs)

	results := make([]interface{}, numFuncs)
	errors := make([]error, numFuncs)
	if p.concurrency > 0 {
		sem = make(chan struct{}, p.concurrency)
	} else {
		sem = make(chan struct{}, numFuncs)
	}

	wg.Add(numFuncs)

	for i, fn := range funcs {
		sem <- struct{}{} // Acquire semaphore

		go func(i int, fn func() chan ResErr) {
			defer func() {
				wg.Done()
				<-sem // Release semaphore
			}()

			ch := fn()

			select {
			case <-ctx.Done():
				errors[i] = ctx.Err()

			case resErr := <-ch:
				results[i] = resErr.Res
				errors[i] = resErr.Err

				switch resErr.Err.(type) {
				// One of the paralyzable functions panicked.
				// Catch it here and re-panic in the main go-routine.
				case *ErrPanic:
					e, ok := resErr.Err.(*ErrPanic)
					if ok {
						panikOnce.Do(func() {
							panik = e.panik
						})
					}
				}
			}
		}(i, convert(fn))
	}

	wg.Wait()

	if panik != nil {
		panic(panik)
	}

	return results, errors
}
