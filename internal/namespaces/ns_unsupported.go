// +build !linux !cgo

package namespaces

func HandleNamespaces(cmd, target string) error {
	return nil
}
