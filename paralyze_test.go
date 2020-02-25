package paralyze

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	someError = errors.New("some error")

	slowFn  = func() (interface{}, error) { time.Sleep(time.Second); return "ok", nil }
	fastFn  = func() (interface{}, error) { return 55, nil }
	errFn   = func() (interface{}, error) { return nil, someError }
	panicFn = func() (interface{}, error) { panic("whoops") }
)

func TestParalyze(t *testing.T) {
	results, errs := Paralyze(slowFn, fastFn, errFn)

	// Make sure both slices returned are the correct length
	assert.Equal(t, 3, len(results))
	assert.Equal(t, 3, len(errs))

	// Assert that return values are in the correct order
	assert.Equal(t, "ok", results[0])
	assert.Equal(t, 55, results[1])
	assert.Nil(t, results[2])

	// Assert that errors are
	assert.Nil(t, errs[0])
	assert.Nil(t, errs[1])
	assert.Equal(t, someError, errs[2])
}

func TestParalyzeWithTimeout(t *testing.T) {
	results, errs := ParalyzeWithTimeout(time.Second/2, slowFn, fastFn, errFn)

	// Make sure both slices returned are the correct length
	assert.Equal(t, 3, len(results))
	assert.Equal(t, 3, len(errs))

	// Assert that return values are in the correct order
	assert.Nil(t, results[0])
	assert.Equal(t, 55, results[1])
	assert.Nil(t, results[2])

	// Assert that errors are
	assert.Equal(t, ErrTimedOut, errs[0])
	assert.Nil(t, errs[1])
	assert.Equal(t, someError, errs[2])
}

func TestParalyzeWithZeroTimeout(t *testing.T) {
	results, errs := ParalyzeWithTimeout(0, slowFn, fastFn, errFn)

	// Make sure both slices returned are the correct length
	assert.Equal(t, 3, len(results))
	assert.Equal(t, 3, len(errs))

	// Assert that return values are in the correct order
	assert.Equal(t, "ok", results[0])
	assert.Equal(t, 55, results[1])
	assert.Nil(t, results[2])

	// Assert that errors are
	assert.Nil(t, errs[0])
	assert.Nil(t, errs[1])
	assert.Equal(t, someError, errs[2])
}

func TestParalyzeWithCancel(t *testing.T) {
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(time.Millisecond):
			done <- struct{}{}
		}
	}()

	results, errs := ParalyzeWithCancel(done, slowFn, fastFn, errFn)

	// Make sure both slices returned are the correct length
	assert.Equal(t, 3, len(results))
	assert.Equal(t, 3, len(errs))

	// Assert that return values are in the correct order
	assert.Nil(t, results[0])
	assert.Equal(t, 55, results[1])
	assert.Nil(t, results[2])

	// Assert that errors are
	assert.Equal(t, ErrCanceled, errs[0])
	assert.Nil(t, errs[1])
	assert.Equal(t, someError, errs[2])

}

func TestParalyzeWithCtx(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results, errors := ParalyzeWithContext(
		ctx,
		fnCreator(500*time.Millisecond),
		fnCreator(1200*time.Millisecond),
	)

	assert.Equal(t, "success", results[0])
	assert.NoError(t, errors[0])

	assert.Nil(t, results[1])
	assert.Error(t, errors[1])
}

func TestParalyzeM(t *testing.T) {
	errBadThing := errors.New("bad thing")

	results := ParalyzeM(map[string]Paralyzable{
		"foo": func() (interface{}, error) {
			return "foo", nil
		},
		"err": func() (interface{}, error) {
			return nil, errBadThing
		},
	})

	assert.Equal(t, "foo", results["foo"].Res, "foo")
	assert.Nil(t, results["foo"].Err)
	assert.Nil(t, results["err"].Res)
	assert.Equal(t, errBadThing, results["err"].Err)
}

func fnCreator(wait time.Duration) ParalyzableCtx {
	return func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(wait):
			return "success", nil
		case <-ctx.Done():
			// clean up resources
			return nil, fmt.Errorf("timed out")
		}
	}
}

func TestParalyzePanic(t *testing.T) {
	assert.Panics(t, func() {
		Paralyze(panicFn)
	})
}

func TestParalyzeLimit(t *testing.T) {
	results, errs := ParalyzeLimit(2, slowFn, fastFn, errFn)

	// Make sure both slices returned are the correct length
	assert.Equal(t, 3, len(results))
	assert.Equal(t, 3, len(errs))

	// Assert that return values are in the correct order
	assert.Equal(t, "ok", results[0])
	assert.Equal(t, 55, results[1])
	assert.Nil(t, results[2])

	// Assert that errors are
	assert.Nil(t, errs[0])
	assert.Nil(t, errs[1])
	assert.Equal(t, someError, errs[2])
}
