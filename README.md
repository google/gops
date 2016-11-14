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

For processes that contains the diagnostics agent, gops can report
additional information such as the current stack trace, Go version, memory
stats, etc.

In order to start the diagnostics agent, see the [hello example](https://github.com/google/gops/blob/master/examples/hello/main.go).

Please note that diagnostics features are only supported on Unix-like systems for now.
We are planning to add Windows support in the near future.

### stack
Prints the stack trace for the process identified with the specified PID.
```
$ gops stack -p=<pid>
goroutine 35 [running]:
github.com/google/gops/agent.handle(0x11897a0, 0xc4200d0000, 0xc4200c6000, 0x1, 0x1, 0x0, 0x0)
	/Users/jbd/src/github.com/google/gops/agent/agent.go:63 +0x182
github.com/google/gops/agent.init.1.func2(0x1189140, 0xc420078450)
	/Users/jbd/src/github.com/google/gops/agent/agent.go:50 +0x242
created by github.com/google/gops/agent.init.1
	/Users/jbd/src/github.com/google/gops/agent/agent.go:56 +0x240

goroutine 1 [sleep]:
time.Sleep(0x34630b8a000)
	/Users/jbd/go/src/runtime/time.go:59 +0xf7
main.main()
	/Users/jbd/src/github.com/google/gops/examples/hello/main.go:14 +0x30

goroutine 17 [syscall, locked to thread]:
runtime.goexit()
	/Users/jbd/go/src/runtime/asm_amd64.s:2184 +0x1

goroutine 20 [syscall]:
os/signal.signal_recv(0x0)
	/Users/jbd/go/src/runtime/sigqueue.go:116 +0xff
os/signal.loop()
	/Users/jbd/go/src/os/signal/signal_unix.go:22 +0x22
created by os/signal.init.1
	/Users/jbd/go/src/os/signal/signal_unix.go:28 +0x41

goroutine 21 [select, locked to thread]:
runtime.gopark(0x1114a80, 0x0, 0x110d5ec, 0x6, 0x18, 0x2)
	/Users/jbd/go/src/runtime/proc.go:261 +0x13a
runtime.selectgoImpl(0xc42003ff50, 0x0, 0x18)
	/Users/jbd/go/src/runtime/select.go:423 +0x1307
runtime.selectgo(0xc42003ff50)
	/Users/jbd/go/src/runtime/select.go:238 +0x1c
runtime.ensureSigM.func1()
	/Users/jbd/go/src/runtime/signal_unix.go:408 +0x265
runtime.goexit()
	/Users/jbd/go/src/runtime/asm_amd64.s:2184 +0x1

goroutine 34 [chan receive]:
github.com/google/gops/agent.init.1.func1(0xc4200740c0, 0xc4200880e0, 0x13)
	/Users/jbd/src/github.com/google/gops/agent/agent.go:33 +0x40
created by github.com/google/gops/agent.init.1
	/Users/jbd/src/github.com/google/gops/agent/agent.go:36 +0x214
```

## gc

Runs garbage collector and blocks until the garbage collection is completed.

```
$ gops gc -p=<pid>
```

## memstats

Reports the memory stats from the targetted Go process.

```
$ gops memstats -p=<pid>
alloc: 1265448 bytes
total-alloc: 58395932600 bytes
sys: 12196088 bytes
lookups: 15
mallocs: 1794071
frees: 1793640
heap-alloc: 1265448 bytes
heap-sys: 7569408 bytes
heap-idle: 6012928 bytes
heap-in-use: 1556480 bytes
heap-released: 16384 bytes
heap-objects: 431
stack-in-use: 819200 bytes
stack-sys: 819200 bytes
next-gc: when heap-alloc >= 4194304 bytes
last-gc: 1479100860500328811 ns ago
gc-pause: 856980908 ns
num-gc: 14274
enable-gc: true
debug-gc: false
```

## version

Reports the Go version used to build the target program.

```
$ gops version -p=<pid>
devel +4141054 Thu Nov 3 17:42:01 2016 +0000
```
