paralyze
========
parallelize things

how to get
------------

    go get github.com/gocql/gocql

how to do
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
