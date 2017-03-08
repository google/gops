package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"

	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
	"github.com/pkg/errors"
)

var clientHTTP = http.DefaultClient

var cmds = map[string](func(addr *url.URL) error){
	"stack":      stackTrace,
	"gc":         gc,
	"memstats":   memStats,
	"version":    version,
	"pprof-heap": pprofHeap,
	"pprof-cpu":  pprofCPU,
	"stats":      stats,
	"trace":      trace,
}

func stackTrace(u *url.URL) error {
	return requestWithPrint(u, &signal.Command{
		Code: signal.StackTrace,
	})
}

func gc(u *url.URL) error {
	_, err := request(clientHTTP, u, &signal.Command{
		Code: signal.GC,
	})
	return err
}

func memStats(u *url.URL) error {
	return requestWithPrint(u, &signal.Command{
		Code: signal.MemStats,
	})
}

func version(u *url.URL) error {
	return requestWithPrint(u, &signal.Command{
		Code: signal.Version,
	})
}

func pprofHeap(u *url.URL) error {
	return pprof(u, &signal.Command{
		Code: signal.HeapProfile,
	})
}

func pprofCPU(u *url.URL) error {
	fmt.Println("Profiling CPU now, will take 30 secs...")
	return pprof(u, &signal.Command{
		Code: signal.CPUProfile,
	})
}

func trace(u *url.URL) error {
	fmt.Println("Tracing now, will take 5 secs...")
	out, err := request(clientHTTP, u, &signal.Command{
		Code: signal.Trace,
	})
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return errors.New("nothing has traced")
	}
	tmpfile, err := ioutil.TempFile("", "trace")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name())
	if err := ioutil.WriteFile(tmpfile.Name(), out, 0); err != nil {
		return err
	}
	cmd := exec.Command("go", "tool", "trace", tmpfile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pprof(u *url.URL, c *signal.Command) error {

	tmpDumpFile, err := ioutil.TempFile("", "profile")
	if err != nil {
		return err
	}
	{
		out, err := request(clientHTTP, u, c)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return errors.New("failed to read the profile")
		}
		defer os.Remove(tmpDumpFile.Name())
		if err := ioutil.WriteFile(tmpDumpFile.Name(), out, 0); err != nil {
			return err
		}
	}
	// Download running binary
	tmpBinFile, err := ioutil.TempFile("", "binary")
	if err != nil {
		return err
	}
	{

		out, err := request(clientHTTP, u, &signal.Command{
			Code: signal.BinaryDump,
		})
		if err != nil {
			return errors.New("couldn't retrieve running binary's dump")
		}
		if len(out) == 0 {
			return errors.New("failed to read the binary")
		}
		defer os.Remove(tmpBinFile.Name())
		if err := ioutil.WriteFile(tmpBinFile.Name(), out, 0); err != nil {
			return err
		}
	}
	cmd := exec.Command("go", "tool", "pprof", tmpBinFile.Name(), tmpDumpFile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func stats(u *url.URL) error {
	return requestWithPrint(u, &signal.Command{
		Code: signal.Stats,
	})
}

func requestWithPrint(u *url.URL, c *signal.Command) error {
	out, err := request(clientHTTP, u, c)
	if err != nil {
		return err
	}

	fmt.Printf("%s", out)
	return nil
}

// targetToAddr tries to parse the target string, be it remote request URL
// or local process's PID.
func targetToAddr(target string) (*url.URL, error) {
	if pid, parseErr := strconv.Atoi(target); parseErr == nil {
		port, err := internal.GetPort(pid)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't get port config for given PID")
		}

		u := &url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:" + port,
		}

		return u, nil
	}

	u, err := url.Parse(target)
	return u, err
}

func request(client *http.Client, u *url.URL, c *signal.Command) ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	return body, err
}
