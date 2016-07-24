package paralyze

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/net/context"
)

// Paralyzable is a type of function that can be paralyzed. Since most
// functions don't carry this signature, a common pattern is to wrap an
// existing function in a Paralyzable function.
type Paralyzable func() (interface{}, error)

// ParalyzableCtx is the same as a Paralyzable function, except it accepts a
// context.Context. Functions that implement this type should respect the
// documentation found here: https://godoc.org/golang.org/x/net/context
type ParalyzableCtx func(context.Context) (interface{}, error)

// General errors that can be returned from the paralyze package
var (
	ErrTimedOut = errors.New("timed out")
	ErrCanceled = errors.New("canceled")
)

// Paralyze parallelizes a function and returns a slice containing results and
// a slice containing errors. The results at each index are not mutually exclusive,
// that is if results[i] is not nil, errors[i] is not guaranteed to be nil.
func Paralyze(funcs ...Paralyzable) (results []interface{}, errors []error) {
	var wg sync.WaitGroup
	results = make([]interface{}, len(funcs))
	errors = make([]error, len(funcs))
	wg.Add(len(funcs))

	for i, fn := range funcs {
		go func(i int, fn Paralyzable) {
			defer wg.Done()
			results[i], errors[i] = fn()
		}(i, fn)
	}
	wg.Wait()
	return results, errors
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

	cancel := make(chan struct{})
	go time.AfterFunc(timeout, func() { close(cancel) })

	results, errors := ParalyzeWithCancel(cancel, funcs...)
	for i, err := range errors {
		if err == ErrCanceled {
			errors[i] = ErrTimedOut
		}
	}
	return results, errors
}

// ParalyzeWithCancel does the same as Paralyze, but it accepts a channel that
// allows the function to respond before the paralyzed functions are finished.
// Any functions that are still oustanding will have errors set as ErrCanceled.
func ParalyzeWithCancel(cancel <-chan struct{}, funcs ...Paralyzable) ([]interface{}, []error) {
	var wg sync.WaitGroup
	results := make([]interface{}, len(funcs))
	errors := make([]error, len(funcs))
	wg.Add(len(funcs))

	for i, fn := range funcs {
		go func(i int, fn func() chan ResErr) {
			defer wg.Done()
			ch := fn()
			select {
			case resErr := <-ch:
				results[i] = resErr.Res
				errors[i] = resErr.Err
			case <-cancel:
				errors[i] = ErrCanceled
			}
		}(i, convert(fn))
	}
	wg.Wait()
	return results, errors
}

// ParalyzeWithContext takes a slice of functions that accept a
// context.Context. These functions are responsible for releasing resources
// (closing connections, etc.) and should respect ctx.Done().
func ParalyzeWithContext(ctx context.Context, funcs ...ParalyzableCtx) ([]interface{}, []error) {
	var wg sync.WaitGroup
	results := make([]interface{}, len(funcs))
	errors := make([]error, len(funcs))

	wg.Add(len(funcs))
	for i, fn := range funcs {
		go func(i int, fn ParalyzableCtx) {
			defer wg.Done()
			results[i], errors[i] = fn(ctx)
		}(i, fn)
	}
	wg.Wait()
	return results, errors
}

func convert(fn func() (interface{}, error)) func() chan ResErr {
	return func() chan ResErr {
		ch := make(chan ResErr, 1)
		go func() {
			res, err := fn()
			ch <- ResErr{res, err}
		}()
		return ch
	}
}
