package multierr

import (
	"errors"
	"testing"
)

func TestAppend(t *testing.T) {
	e1 := errors.New("one")
	e2 := errors.New("two")
	e3 := errors.New("three")

	if got := Append(nil, nil); got != nil {
		t.Fatalf("Append(nil, nil) = %v, want nil", got)
	}
	if got := Append(nil, e1); got != e1 {
		t.Fatalf("Append(nil, e1) = %v, want e1", got)
	}

	base := Combine(e1, e2)
	res := Append(base, nil, e3)
	errs := Errors(res)
	if len(errs) != 3 || errs[0] != e1 || errs[1] != e2 || errs[2] != e3 {
		t.Fatalf("Append flattened errs = %v, want [e1 e2 e3]", errs)
	}
}

func TestCombine(t *testing.T) {
	e1 := errors.New("one")
	e2 := errors.New("two")
	e3 := errors.New("three")

	if Combine(nil, nil) != nil {
		t.Fatalf("Combine of only nils should be nil")
	}
	if Combine(nil, e1) != e1 {
		t.Fatalf("Combine should return single error when only one non-nil")
	}

	err := Combine(e1, Combine(e2, e3), nil)
	errs := Errors(err)
	if len(errs) != 3 || errs[0] != e1 || errs[1] != e2 || errs[2] != e3 {
		t.Fatalf("Combine flattened errs = %v, want [e1 e2 e3]", errs)
	}
}

func TestErrors(t *testing.T) {
	if Errors(nil) != nil {
		t.Fatalf("Errors(nil) should be nil")
	}

	e1 := errors.New("one")
	list := Errors(e1)
	if len(list) != 1 || list[0] != e1 {
		t.Fatalf("Errors(single) = %v, want [e1]", list)
	}

	err := Combine(e1, errors.New("two"))
	list = Errors(err)
	if len(list) != 2 || list[0] != e1 || list[1].Error() != "two" {
		t.Fatalf("Errors(multi) = %v, want [e1 e2]", list)
	}

	// ensure returned slice is a copy
	list[0] = errors.New("changed")
	list2 := Errors(err)
	if len(list2) != 2 || list2[0] != e1 {
		t.Fatalf("modifying returned slice affected underlying errors: %v", list2)
	}
}
