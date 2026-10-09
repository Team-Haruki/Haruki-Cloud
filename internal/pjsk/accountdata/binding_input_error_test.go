package accountdata

import (
	"testing"

	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestBindingIndexErrorsAreUserInput(t *testing.T) {
	items := []BindingListItem{{}}
	_, err := bindingAtIndex(items, 3, "cn")
	testutil.RequireUserError(t, err, usererror.CodeOutOfRange, "binding.selector_index_out_of_range_region")
	_, err = bindingAtIndex(items, 2, "")
	testutil.RequireUserError(t, err, usererror.CodeOutOfRange, "binding.selector_index_out_of_range")
}
