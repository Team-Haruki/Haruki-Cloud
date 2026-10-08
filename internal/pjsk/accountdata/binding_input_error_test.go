package accountdata

import (
	"testing"

	"haruki-cloud/utils/usererror"
)

func TestBindingIndexErrorsAreUserInput(t *testing.T) {
	items := []BindingListItem{{}}
	_, err := bindingAtIndex(items, 3, "cn")
	if err == nil || err.Error() != "指定的CN服账号序号超出范围，目前仅绑定了1个账号" || !usererror.IsInput(err) {
		t.Fatalf("server index error = %v (input %v)", err, usererror.IsInput(err))
	}
	_, err = bindingAtIndex(items, 2, "")
	if err == nil || err.Error() != "指定的账号序号超出范围，目前仅绑定了1个账号" || !usererror.IsInput(err) {
		t.Fatalf("index error = %v (input %v)", err, usererror.IsInput(err))
	}
}
