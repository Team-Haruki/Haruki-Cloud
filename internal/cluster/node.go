package cluster

import (
	"haruki-cloud/config"
	"haruki-cloud/utils/usererror"
)

// ErrReadOnly is returned by every write while the node is read-only. It is
// a typed user error (usererror.CodeReadOnly); compare with errors.Is.
var ErrReadOnly error = usererror.ReadOnly()

func IsReadOnly() bool {
	return config.Cfg.Node.ReadOnly
}

func EnsureWritable(readOnly bool) error {
	if readOnly {
		return ErrReadOnly
	}
	return nil
}
