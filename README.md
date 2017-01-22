# gops [![Build Status](https://travis-ci.org/google/gops.svg?branch=master)](https://travis-ci.org/google/gops) [![GoDoc](https://godoc.org/github.com/google/gops/agent?status.svg)](https://godoc.org/github.com/google/gops/agent)

gops is a command to list and diagnose Go processes currently running on your system.

```
$ gops
983     uplink-soecks	(/usr/local/bin/uplink-soecks)
52697   gops	(/Users/jbd/bin/gops)
4132*   foops (/Users/jbd/bin/foops)
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
	if err := agent.Listen(nil); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Hour)
}
```

### Diagnostics manual

It is possible to use gops tool both in local and remote mode.

Local mode requires that you start the target binary as the same user that runs gops binary.
To use gops in a remote mode you need to know target binary's agent's host and port.

In Local mode use process's PID as a `<target>`; in Remote mode `<target>` is `host:port` combination.

#### 0. listing all processes running locally

To print all go processes, run `gops` without arguments:

```sh
$ gops
983     uplink-soecks	(/usr/local/bin/uplink-soecks)
52697   gops	(/Users/jbd/bin/gops)
4132*   foops (/Users/jbd/bin/foops)
51130   gocode	(/Users/jbd/bin/gocode)
```

Note that processes running the agent are marked with `*` next to the PID (e.g. `4132*`).

#### 1. stack

In order to print the current stack trace from a target program, run the following command:

```sh
$ gops stack <target>
```

#### 2. memstats

To print the current memory stats, run the following command:

```sh
$ gops memstats <target>
```

#### 3. pprof

gops supports CPU and heap pprof profiles. After reading either heap or CPU profile,
it shells out to the `go tool pprof` and let you interatively examine the profiles.

To enter the CPU profile, run:

```sh
$ gops pprof-cpu <target>
```

To enter the heap profile, run:

```sh
$ gops pprof-heap <target>
```

#### 4.  gc

If you want to force run garbage collection on the target program, run the following command.
It will block until the GC is completed.

```sh
$ gops gc <target>
```

#### 5. version

gops reports the Go version the target program is built with, if you run the following:

```sh
$ gops version <target>
```

#### 6. stats

To print the runtime statistics such as number of goroutines and `GOMAXPROCS`, run the following:

```sh
$ gops stats <target>
```
