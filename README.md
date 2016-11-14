# gops [![Build Status](https://travis-ci.org/google/gops.svg?branch=master)](https://travis-ci.org/google/gops)

gops is a command to list and diagnose Go processes currently running on your system.

```
$ gops
983     uplink-soecks	(/usr/local/bin/uplink-soecks)
52697   gops	(/Users/jbd/bin/gops)
51130   gocode	(/Users/jbd/bin/gocode)
```

## Installation

```
$ go get -u github.com/google/gops
```

## Diagnostics

For processes that starts the diagnostics agent, gops can report
additional information such as the current stack trace, Go version, memory
stats, etc.

In order to start the diagnostics agent, see the [hello example](https://github.com/google/gops/blob/master/examples/hello/main.go).

``` go
package main

import (
	"log"
	"time"

	"github.com/google/gops/agent"
)

func main() {
	if err := agent.Start(); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Hour)
}
```

Please note that diagnostics features are only supported on Unix-like systems for now.
We are planning to add Windows support in the near future.

### Diagnostics manual


#### 1. stack

In order to print the current stack trace from a target program, run the following command:

```sh
$ gops stack -p=<PID>

```

#### 2. memstats

To print the current memory stats, run the following command:

```sh
$ gops memstats -p=<PID>
```

#### 3. pprof

gops supports CPU and heap pprof profiles. After reading either heap or CPU profile,
it shells out to the `go tool pprof` and let you interatively examine the profiles.

To enter the CPU profile, run:

```sh
$ gops pprof-cpu -p=<PID>
```

To enter the heap profile, run:

```sh
$ gops pprof-mem -p=<PID>
```

#### 4.  gc

If you want to force run garbage collection on the target program, run the following command.
It will block until the GC is completed.

```sh
$ gops gc -p=<PID>
```

#### 5. version

gops reports the Go version the target program is built with, if you run the following:

```sh
$ gops version -p=<PID>
```

