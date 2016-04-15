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
	ErrTimedOut       = errors.New("timed out")
	ErrCanceled       = errors.New("canceled")
	ErrInvalidTimeout = errors.New("invalid timeout")

	errParalyzeError = errors.New("internal problem with paralyze, please file an issue")
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

// ParalyzeM parallelizes a map of strings to functions. The return type is a
// map of keys to a map containing two keys: res and err.
func ParalyzeM(m map[string]Paralyzable) map[string]map[string]interface{} {
	var names []string
	var fns []Paralyzable

	for name, fn := range m {
		names = append(names, name)
		fns = append(fns, fn)
	}

	res := make(map[string]map[string]interface{})
	results, errs := Paralyze(fns...)
	for i := range results {
		name := names[i]
		res[name] = make(map[string]interface{})
		res[name]["res"] = results[i]
		res[name]["err"] = errs[i]
	}

	return res
}

// ParalyzeWithTimeout does the same as Paralyze, but it accepts a timeout. If
// the timeout is exceeded before all paralyzed functions are complete, the
// results will be discarded and errors will be set with the value ErrTimedOut.
func ParalyzeWithTimeout(timeout time.Duration, funcs ...Paralyzable) ([]interface{}, []error) {
	cancel := make(chan struct{})
	if timeout == 0 {
		return ParalyzeWithCancel(cancel, funcs...)
	}
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
		go func(i int, fn func() (chan interface{}, chan error)) {
			defer wg.Done()
			resCh, errCh := fn()
			select {
			case res := <-resCh:
				results[i] = res
			case err := <-errCh:
				errors[i] = err
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

// Speculate calls each of the functions provided until one of them succeeds.
// After specifies the time to wait before calling the next function. If
// fallbackOnError is enabled, a function that returns an error will be
// ignored. If fallbackOnError is enabled and all functions fail, the last
// returned error is returned.
func Speculate(after time.Duration, fallbackOnError bool, fn Paralyzable, fallbacks ...Paralyzable) (interface{}, error) {
	if after <= 0 {
		return nil, ErrInvalidTimeout
	}

	t := time.NewTicker(after)
	defer t.Stop()

	fns := append([]Paralyzable{fn}, fallbacks...)
	ch := make(chan resErr, len(fns))

	for i, fn := range fns {
		// sigh
		i, fn := i, fn

		go func() {
			ch <- makeReq(fn)
		}()

		select {
		case resErr := <-ch:
			if fallbackOnError {
				if resErr.err != nil {
					if i < len(fns) {
						continue
					}
				}
			}
			return resErr.res, resErr.err
		case <-t.C:
			continue
		}
	}

	// This line should never be touched
	return nil, errParalyzeError
}

func makeReq(fn Paralyzable) resErr {
	res, err := fn()
	return resErr{res, err}
}

type resErr struct {
	res interface{}
	err error
}

func convert(fn func() (interface{}, error)) func() (chan interface{}, chan error) {
	return func() (chan interface{}, chan error) {
		resCh := make(chan interface{})
		errCh := make(chan error)
		go func() {
			res, err := fn()
			if err != nil {
				errCh <- err
			} else {
				resCh <- res
			}
		}()
		return resCh, errCh
	}
}
