package paralyze

import "sync"

type Paralyzable func() (interface{}, error)

// Paralyze parallelizes a function and returns two slices.
func Paralyze(funcs ...Paralyzable) ([]interface{}, []error) {
	var wg sync.WaitGroup
	results := make([]interface{}, len(funcs))
	errors := make([]error, len(funcs))

	wg.Add(len(funcs))

	for i, fn := range funcs {
		go func(i int, fn Paralyzable) {
			defer wg.Done()
			if res, err := fn(); err != nil {
				errors[i] = err
			} else {
				results[i] = res
			}
		}(i, fn)
	}

	wg.Wait()
	return results, errors
}
