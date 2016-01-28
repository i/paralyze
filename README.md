paralyze
========
parallelize things

how to get
------------

    go get github.com/i/paralyze

vanilla parallelization
---------

```go
package main

import (
  "fmt"
  "time"

  "github.com/i/paralyze"
)

func main() {
	fn1 := func() (interface{}, error) {
		time.Sleep(time.Second)
		return "OK!", nil
	}
	fn2 := func() (interface{}, error) {
		time.Sleep(time.Second)
		return "RAD!", nil
	}
	fn3 := func() (interface{}, error) {
		return nil, fmt.Errorf("failure!")
	}

	results, errs := paralyze.Paralyze(fn1, fn2, fn3)
	fmt.Println(results) // prints [ OK! RAD! <nil>]
	fmt.Println(errs)    // prints [<nil> <nil> failure!]
}

```

parallelization with a map
---------

```go
package main

import (
  "fmt"
  "time"

  "github.com/i/paralyze"
)

func main() {
  results := ParalyzeM(map[string]Paralyzable{
    "goodie": func() (interface{}, error) {
      return "wowe!", nil
    },
    "bad": func() (interface{}, error) {
      return nil, fmt.Errorf("you're the worst")
    },
  })

  fmt.Println(results["goodie"]["res"]) // prints wowe!
  fmt.Println(results["goodie"]["err"]) // prints <nil>
  fmt.Println(results["bad"]["res"])    // prints <nil>
  fmt.Println(results["bad"]["err"])    // prints you're the worst!
}

```

parallelization with timeouts
---------

```go
package main

import (
  "fmt"
  "time"

  "github.com/i/paralyze"
)

func sleepAndSay(t time.Duration) paralyze.Paralyzable{
  return func() (interface{}, error) {
    time.Sleep(t)
    return fmt.Sprintf("I waited for this long: %v", t), nil
  }
}

func main() {
  results, errs := paralyze.ParalyzeWithTimeout(
    time.Second, // wait for one second before giving up
    sleepAndSay(500*time.Millisecond),
    sleepAndSay(2*time.Second),
  )
  fmt.Println(results) // prints [I waited for this long: 500ms <nil>]
  fmt.Println(errs)    // prints [<nil> timed out]
}

```

parallelization using context
---------

```go
package main

import (
  "fmt"
  "time"

  "golang.org/x/net/context"
  "golang.org/x/net/context/ctxhttp"

  "github.com/i/paralyze"
)

func newRequestMaker(url string) ParalyzableCtx {
  return func(ctx context.Context) (interface{}, error) {
    return ctxhttp.Get(ctx, nil, url)
  }
}

func main() {
  ctx, cancel := context.WithTimeout(context.Background(), time.Second)
  defer cancel()

  responses, errors := paralyze.ParalyzeWithContext(
    ctx,
    newRequestMaker("https://google.com"),
    newRequestMaker("http://rms.sexy"),
  )

  // do something great
}

```
