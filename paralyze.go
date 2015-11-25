package paralyze

import "sync"

type Paralyzable func() (interface{}, error)

// Paralyze parallelizes a function and returns two slices.
func Paralyze(funcs ...Paralyzable) ([]interface{}, []error) {
	var wg sync.WaitGroup
	results := make([]interface{}, len(funcs))
	errors := make([]error, len(funcs))
	for i, fn := range funcs {
		wg.Add(1)
		go func(i int, fn func() (chan interface{}, chan error)) {
			defer wg.Done()
			resCh, errCh := fn()
			select {
			case res := <-resCh:
				results[i] = res
			case err := <-errCh:
				errors[i] = err
			}
		}(i, convert(fn))
	}
	wg.Wait()
	return results, errors
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
