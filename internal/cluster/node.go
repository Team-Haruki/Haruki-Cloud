package cluster

import (
	"errors"

	"haruki-cloud/config"
)

const ReadOnlyMessage = "当前 Cloud 节点处于只读模式，暂时不能修改用户数据，请稍后重试"

var ErrReadOnly = errors.New(ReadOnlyMessage)

func IsReadOnly() bool {
	return config.Cfg.Node.ReadOnly
}

func EnsureWritable(readOnly bool) error {
	if readOnly {
		return ErrReadOnly
	}
	return nil
}
