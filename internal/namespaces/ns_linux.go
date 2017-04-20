// +build linux,cgo

package namespaces

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/kardianos/osext"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/opencontainers/runc/libcontainer/user"
	"github.com/vishvananda/netlink/nl"
)

const (
	libContainerPipeEnv = "_LIBCONTAINER_INITPIPE"
)

func HandleNamespaces(cmd, target string) error {
	if !targetIsPID(target) {
		return nil
	}
	if os.Getuid() != 0 {
		return nil
	}
	var err error
	if inNamespace() {
		err = setupHomeEnv(target)
	} else {
		err = reexecInNamespace(cmd, target)
		if err == nil {
			os.Exit(0)
		}
	}
	return err
}

func targetIsPID(target string) bool {
	if strings.Index(target, ":") == -1 {
		_, err := strconv.Atoi(target)
		return err == nil
	}
	return false
}

func inNamespace() bool {
	_, hasInitPipe := os.LookupEnv(libContainerPipeEnv)
	return hasInitPipe
}

func reexecInNamespace(command, pid string) error {
	path, err := osext.Executable()
	if err != nil {
		return err
	}
	nsPid, err := parseNSPid(pid)
	if err != nil {
		return err
	}
	parent, child, err := newPipe()
	if err != nil {
		return err
	}
	namespaces := []string{
		fmt.Sprintf("mnt:/proc/%s/ns/mnt", pid),
		fmt.Sprintf("net:/proc/%s/ns/net", pid),
		fmt.Sprintf("pid:/proc/%s/ns/pid", pid),
		fmt.Sprintf("ipc:/proc/%s/ns/ipc", pid),
		fmt.Sprintf("uts:/proc/%s/ns/uts", pid),
	}
	cmd := &exec.Cmd{
		Path:       path,
		Args:       []string{os.Args[0], command, nsPid},
		ExtraFiles: []*os.File{child},
		Env: []string{
			libContainerPipeEnv + "=3",
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	r := nl.NewNetlinkRequest(int(libcontainer.InitMsg), 0)
	r.AddData(&libcontainer.Bytemsg{
		Type:  libcontainer.NsPathsAttr,
		Value: []byte(strings.Join(namespaces, ",")),
	})
	if _, err = io.Copy(parent, bytes.NewReader(r.Serialize())); err != nil {
		return err
	}
	decoder := json.NewDecoder(parent)
	var childPid struct {
		Pid int `json:"pid"`
	}
	if err = cmd.Wait(); err != nil {
		return err
	}
	if err = decoder.Decode(&childPid); err != nil {
		return err
	}
	p, err := os.FindProcess(childPid.Pid)
	if err != nil {
		return err
	}
	p.Wait()
	return nil
}

func newPipe() (parent *os.File, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

var nsPidRegex = regexp.MustCompile(`(?sm).*NSpid:.*?\s+(\d+)$`)

func parseNSPid(pid string) (string, error) {
	data, err := ioutil.ReadFile(fmt.Sprintf("/proc/%s/status", pid))
	if err != nil {
		return "", err
	}
	result := nsPidRegex.FindStringSubmatch(string(data))
	if err != nil {
		return "", err
	}
	if len(result) != 2 {
		return pid, nil
	}
	return result[1], nil
}

func setupHomeEnv(pid string) error {
	info, err := os.Stat(fmt.Sprintf("/proc/%s", pid))
	if err != nil {
		return err
	}
	uid := uint32(0)
	if statData, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = statData.Uid
	}
	defaultExecUser := user.ExecUser{
		Uid:  0,
		Gid:  0,
		Home: "/",
	}
	passwdPath, err := user.GetPasswdPath()
	if err != nil {
		return err
	}
	groupPath, err := user.GetGroupPath()
	if err != nil {
		return err
	}
	execUser, err := user.GetExecUserPath(strconv.Itoa(int(uid)), &defaultExecUser, passwdPath, groupPath)
	if err != nil {
		return err
	}
	if envHome := os.Getenv("HOME"); envHome == "" {
		if err := os.Setenv("HOME", execUser.Home); err != nil {
			return err
		}
	}
	if err := system.Setgid(execUser.Gid); err != nil {
		return err
	}
	if err := system.Setuid(execUser.Uid); err != nil {
		return err
	}
	return nil
}
