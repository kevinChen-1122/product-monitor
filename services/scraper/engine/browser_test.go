package engine

import (
	"errors"
	"testing"
)

func TestIsRecoverableBrowserError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("target closed: pipe closed: EOF"), true},
		{errors.New("建立分頁上下文失敗: timeout"), false},
		{nil, false},
	}
	for _, c := range cases {
		if got := IsRecoverableBrowserError(c.err); got != c.want {
			t.Errorf("%q: got %v want %v", c.err, got, c.want)
		}
	}
}
